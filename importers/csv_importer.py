"""
CSV Telemetry Importer for IdoRacing Analytics.
Reads telemetry exported from MoTeC i2, third-party apps or the IdoRacing recorder. Leading
"# key: value" lines carry session metadata (simulator, track, car), as the recorder writes them.
"""

import os
import re
from datetime import datetime

import numpy as np
import pandas as pd

from domain.session import Lap, Session, normalize_track_name
from importers.acevo_importer import resolve_acevo_session_info
from importers.laps import distance_covered, truncate_teleport_jumps, without_boundary_wrap
from importers.lmu_importer import resolve_lmu_session_info


class CSVImporter:
    """Importer for CSV-formatted driving telemetry sessions."""

    CANONICAL_COLUMNS = {
        "speed": ["speed", "corr speed", "velocity", "gps speed"],
        "throttle": ["throttle", "throttle pos", "gas", "accelerator"],
        "brake": ["brake", "brake pos", "braking"],
        "gear": ["gear", "gear pos"],
        "lat_accel": ["lat_accel", "g force lat", "g_lat", "glat", "lateral_acceleration"],
        "long_accel": ["long_accel", "g force long", "g_lon", "glong", "longitudinal_acceleration"],
        "time": ["time", "lap time", "elapsed time", "timestamp"],
        "distance": ["distance", "track distance", "dist"],
        "lap_distance": ["lap_distance", "lap distance"],
        "x": ["x", "pos_x", "car coord x"],
        "y": ["y", "pos_y", "car coord y"],
        "lap_valid": ["lap_valid"],
        "steer_angle": ["steer_angle", "steered angle", "steerangle", "steering"],
        "rpm": ["rpm", "rpms", "engine rpm", "engine_rpm"],
        "lap": ["lap", "lap number", "lap_number"],
        "pressure_fl": ["pressure_fl", "press_fl", "wheel_pressure_fl", "tyre_pressure_fl", "pressures_fl"],
        "pressure_fr": ["pressure_fr", "press_fr", "wheel_pressure_fr", "tyre_pressure_fr", "pressures_fr"],
        "pressure_rl": ["pressure_rl", "press_rl", "wheel_pressure_rl", "tyre_pressure_rl", "pressures_rl"],
        "pressure_rr": ["pressure_rr", "press_rr", "wheel_pressure_rr", "tyre_pressure_rr", "pressures_rr"],
        "tc": ["tc", "setup_tc1", "tc1", "traction_control"],
        "abs": ["abs", "setup_abs", "anti_lock_braking"],
        "brake_bias": ["brake_bias", "setup_brake_bias", "bias", "brake_balance"],
    }

    def __init__(self, filepath: str, car: str = "Unknown car", track: str = "Unknown track"):
        self.filepath = filepath
        self.car = car
        self.track = track
        if not os.path.exists(filepath):
            raise FileNotFoundError(f"File not found: {filepath}")

    def parse(self) -> Session:
        """Parse CSV file and construct a Session object."""
        metadata = self._read_metadata()
        filename = os.path.basename(self.filepath)
        session_id = os.path.splitext(filename)[0]

        session_date = datetime.now()
        date_match = re.search(r"(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})", session_id)
        if date_match:
            y, mo, d, h, mi = (int(x) for x in date_match.groups())
            session_date = datetime(y, mo, d, h, mi)

        is_acevo = (
            metadata.get("simulator") == "Assetto Corsa EVO"
            or session_id.startswith("acevo")
            or "assettocorsaevo" in self.filepath.lower()
        )
        acevo_info = {}
        if is_acevo:
            acevo_info = resolve_acevo_session_info(self.filepath, session_date)
            if acevo_info:
                csv_track = metadata.get("track", "")
                if (
                    csv_track
                    and csv_track not in ("", "Unknown track", "Ricardo Tormo de Valencia")
                    and normalize_track_name(csv_track) != normalize_track_name(acevo_info.get("track", ""))
                ):
                    acevo_info = {}
            if acevo_info:
                if not metadata.get("track") or metadata.get("track") in (
                    "",
                    "Unknown track",
                    "Ricardo Tormo de Valencia",
                ):
                    metadata["track"] = acevo_info["track"]
                if not metadata.get("car") or metadata.get("car") in ("", "Unknown car", "Porsche 911 GT3 R (992)"):
                    metadata["car"] = acevo_info["car"]
                metadata["simulator"] = "Assetto Corsa EVO"

        is_lmu = (
            metadata.get("simulator") == "Le Mans Ultimate"
            or session_id.startswith("lmu")
            or "lemansultimate" in self.filepath.lower()
        )
        lmu_info = {}
        if is_lmu:
            lmu_info = resolve_lmu_session_info(self.filepath, session_date)
            if lmu_info:
                csv_track = metadata.get("track", "")
                if (
                    csv_track
                    and csv_track not in ("", "Unknown track")
                    and normalize_track_name(csv_track) != normalize_track_name(lmu_info.get("track", ""))
                ):
                    lmu_info = {}
            if lmu_info:
                if not metadata.get("track") or metadata.get("track") in ("", "Unknown track"):
                    metadata["track"] = lmu_info["track"]
                if lmu_info.get("car"):
                    metadata["car"] = lmu_info["car"]
                metadata["simulator"] = "Le Mans Ultimate"

        df = pd.read_csv(self.filepath, comment="#")

        # Check if row 0 has units (e.g. non-numeric while row 1 is numeric)
        if len(df) > 1 and df.iloc[0].dtype == object:
            try:
                pd.to_numeric(df.iloc[0].dropna())
            except (ValueError, TypeError):
                # Row 0 likely contains units, skip it and reload
                df = pd.read_csv(self.filepath, comment="#", skiprows=[1])

        # Normalize column names
        normalized_cols = {col: col.strip().lower() for col in df.columns}
        df.rename(columns=normalized_cols, inplace=True)

        telemetry: dict[str, list[float]] = {}
        for canonical, variants in self.CANONICAL_COLUMNS.items():
            matched_col = None
            for variant in variants:
                if variant in df.columns:
                    matched_col = variant
                    break
            if matched_col is not None:
                numeric_series = pd.to_numeric(df[matched_col], errors="coerce").fillna(0.0)
                telemetry[canonical] = numeric_series.tolist()

        # Lap distance is a position along the track only when the recorder had a valid
        # track_length_m AND the max lap_distance stays within ~1.2x of it. When
        # track_length_m is 0 (static page not read), lap_distance is accumulated distance
        # from the distanceTracker fallback, not track position.
        meta_track_len = float(metadata.get("track_length_m", 0.0) or 0.0)
        ld_max = max(telemetry.get("lap_distance", [0.0])) if "lap_distance" in telemetry else 0.0
        track_position = (
            "lap_distance" in telemetry
            and ld_max > 100.0
            and not is_acevo
            and meta_track_len > 100.0
            and ld_max <= meta_track_len * 1.2
        )
        # Keep the raw lap_distance channel for stuck-lap detection (it has tracker resets)
        raw_lap_distance = list(telemetry.get("lap_distance", [])) if "lap_distance" in telemetry else []
        if track_position:
            telemetry["distance"] = telemetry.pop("lap_distance")
        else:
            telemetry.pop("lap_distance", None)
            telemetry.pop("distance", None)
        lap_valid = telemetry.pop("lap_valid", None)

        # If required channels are missing, provide sane defaults
        n_samples = len(df)
        if "time" not in telemetry:
            telemetry["time"] = [i * 0.05 for i in range(n_samples)]
        if "distance" not in telemetry and "speed" in telemetry:
            # Approximate distance from speed in km/h: dist += (speed / 3.6) * dt
            times = telemetry["time"]
            speeds = telemetry["speed"]
            dist_acc = 0.0
            dist_list = [0.0]
            for i in range(1, len(times)):
                dt = max(times[i] - times[i - 1], 0.001)
                dist_acc += (speeds[i] / 3.6) * dt
                dist_list.append(dist_acc)
            telemetry["distance"] = dist_list

        for req in ["speed", "throttle", "brake", "gear", "lat_accel", "long_accel"]:
            if req not in telemetry:
                telemetry[req] = [0.0] * n_samples

        # Segment into laps if lap column is present
        laps: list[Lap] = []
        corrupted_laps = False
        stuck_lap_counter = False
        if "lap" in telemetry:
            lap_arr = np.array(telemetry["lap"])
            unique_laps = np.unique(lap_arr)
            dist_series = telemetry.get("distance", [])
            total_dist = dist_series[-1] if len(dist_series) > 0 else 0.0
            # In AC EVO Early Access, the shared memory offset for CompletedLaps contains NormalizedCarPosition (float32)
            # resulting in hundreds/thousands of pseudo-unique lap numbers, or remains stuck on 1 for the whole session.
            if is_acevo and (
                len(unique_laps) > 20
                or np.any(lap_arr < -100)
                or np.any(lap_arr > 10000)
                or (len(unique_laps) <= 1 and total_dist > 4500.0)
            ):
                corrupted_laps = True
            elif len(unique_laps) <= 3 and len(dist_series) > 100 and "speed" in telemetry and "time" in telemetry:
                # ACC practice/qualifying can leave CompletedLaps stuck at 0 (or 1) for the whole
                # session when laps are invalid, so the recorder writes lap=1 (or 1,2) for every
                # sample. Detect this by comparing the unique lap count against the physically
                # integrated distance: if the car covered many more metres than unique_laps *
                # track_length, the lap counter is stuck and we must segment by distance.
                speeds_arr = np.asarray(telemetry["speed"], dtype=np.float64)
                times_arr = np.asarray(telemetry["time"], dtype=np.float64)
                if len(speeds_arr) == len(times_arr) and len(speeds_arr) > 10:
                    phys_total = float(np.sum((speeds_arr[1:] / 3.6) * np.diff(times_arr)))
                    track_len_est = float(metadata.get("track_length_m", 0.0) or 0.0)
                    if track_len_est < 100.0 and track_position:
                        track_len_est = float(np.max(dist_series)) if len(dist_series) > 0 else 0.0
                    if track_len_est > 100.0 and phys_total > track_len_est * (len(unique_laps) + 1) * 0.7:
                        stuck_lap_counter = True
                    elif track_len_est < 100.0 and not track_position and raw_lap_distance:
                        # track_length_m unknown and lap_distance is accumulated: use the raw
                        # lap_distance channel (which has distanceTracker resets) to estimate
                        # track length from the 75th percentile physical span between drops
                        # (filters out short teleport segments).
                        ld_arr = np.asarray(raw_lap_distance, dtype=np.float64)
                        drops_tmp = np.where(np.diff(ld_arr) < -0.5 * float(np.max(ld_arr)))[0]
                        if len(drops_tmp) >= 2:
                            bounds_tmp = [0, *drops_tmp.tolist(), len(ld_arr)]
                            phys_spans = []
                            for a, b in zip(bounds_tmp[:-1], bounds_tmp[1:], strict=True):
                                if b - a > 30:
                                    seg_v = speeds_arr[a:b] / 3.6
                                    seg_t = times_arr[a:b]
                                    phys_spans.append(float(np.sum(seg_v[1:] * np.diff(seg_t))))
                            if phys_spans:
                                track_len_est = float(np.percentile(phys_spans, 75))
                                if track_len_est > 100.0 and phys_total > track_len_est * (len(unique_laps) + 1) * 0.7:
                                    stuck_lap_counter = True
            if stuck_lap_counter:
                d_arr = np.asarray(dist_series, dtype=np.float64)
                track_len_est = float(metadata.get("track_length_m", 0.0) or 0.0)
                if track_len_est < 100.0:
                    if track_position:
                        track_len_est = float(np.max(d_arr)) if len(d_arr) > 0 else 0.0
                    elif raw_lap_distance:
                        # Accumulated distance: estimate track length from 75th percentile
                        # physical span between drops (filters teleport segments)
                        ld_arr = np.asarray(raw_lap_distance, dtype=np.float64)
                        drops_tmp = np.where(np.diff(ld_arr) < -0.5 * float(np.max(ld_arr)))[0]
                        if len(drops_tmp) >= 2:
                            speeds_arr = np.asarray(telemetry["speed"], dtype=np.float64)
                            times_arr = np.asarray(telemetry["time"], dtype=np.float64)
                            bounds_tmp = [0, *drops_tmp.tolist(), len(ld_arr)]
                            phys_spans = []
                            for a, b in zip(bounds_tmp[:-1], bounds_tmp[1:], strict=True):
                                if b - a > 30:
                                    seg_v = speeds_arr[a:b] / 3.6
                                    seg_t = times_arr[a:b]
                                    phys_spans.append(float(np.sum(seg_v[1:] * np.diff(seg_t))))
                            if phys_spans:
                                track_len_est = float(np.percentile(phys_spans, 75))
                        if track_len_est < 100.0:
                            track_len_est = 4000.0
                if track_position:
                    # Track position channel: segment by wraps (position drops from ~track_len to ~0)
                    wrap_threshold = -0.5 * track_len_est if track_len_est > 100.0 else -500.0
                    wrap_indices = np.where(np.diff(d_arr) < wrap_threshold)[0] + 1
                    boundaries = [0, *wrap_indices.tolist(), len(d_arr)]
                elif raw_lap_distance:
                    # Accumulated lap_distance: segment at each large tracker-reset drop.
                    # These drops mark where the recorder's distanceTracker detected a lap
                    # change, which is the most reliable boundary when CompletedLaps is stuck.
                    ld_arr = np.asarray(raw_lap_distance, dtype=np.float64)
                    wrap_indices = np.where(np.diff(ld_arr) < -0.5 * float(np.max(ld_arr)))[0] + 1
                    boundaries = [0, *wrap_indices.tolist(), len(ld_arr)]
                else:
                    # Fallback: segment accumulated distance by multiples of estimated track length
                    boundaries = [0]
                    target = track_len_est
                    for i, d in enumerate(d_arr):
                        if d >= target:
                            boundaries.append(i)
                            target += track_len_est
                    boundaries.append(len(d_arr))
                for seg_i in range(len(boundaries) - 1):
                    start, end = boundaries[seg_i], boundaries[seg_i + 1]
                    if end - start < 30:
                        continue
                    lap_telemetry = {k: [telemetry[k][idx] for idx in range(start, end)] for k in telemetry}
                    t0 = lap_telemetry["time"][0]
                    lap_telemetry["time"] = [v - t0 for v in lap_telemetry["time"]]
                    # When segmenting by raw lap_distance drops (stuck counter), the boundaries
                    # already sit at tracker resets, so keep the full segment: a teleport inside
                    # it (e.g. garage return between laps) must not truncate the lap time.
                    if track_position:
                        lap_telemetry, is_incomplete = truncate_teleport_jumps(lap_telemetry)
                        assert lap_telemetry is not None
                        lap_telemetry["distance"] = without_boundary_wrap(lap_telemetry["distance"])
                    else:
                        is_incomplete = False
                        d0 = lap_telemetry["distance"][0]
                        lap_telemetry["distance"] = [v - d0 for v in lap_telemetry["distance"]]
                    assert lap_telemetry is not None
                    if lap_telemetry.get("distance") and lap_telemetry["distance"][0] < 0:
                        d0 = lap_telemetry["distance"][0]
                        lap_telemetry["distance"] = [v - d0 for v in lap_telemetry["distance"]]
                    t_arr = lap_telemetry["time"]
                    lap_time = float(t_arr[-1] - t_arr[0]) if len(t_arr) > 1 else 0.0
                    is_valid = not is_incomplete and lap_time > 20.0
                    if is_valid and lap_valid is not None:
                        lap_valid_samples = [lap_valid[idx] for idx in range(start, min(end, start + len(t_arr)))]
                        valid_fraction = sum(1 for v in lap_valid_samples if v >= 1) / max(len(lap_valid_samples), 1)
                        if valid_fraction >= 0.5:
                            is_valid = all(v >= 1 for v in lap_valid_samples)
                    laps.append(
                        Lap(
                            lap_number=seg_i + 1,
                            lap_time=lap_time,
                            is_valid=is_valid,
                            is_incomplete=is_incomplete,
                            telemetry=lap_telemetry,
                            has_telemetry=True,
                        )
                    )
                if len(laps) > 1:
                    spans = [distance_covered(lap.telemetry or {}) for lap in laps]
                    ref_span = float(np.median(spans))
                    for lap, span in zip(laps, spans, strict=True):
                        if span < 0.7 * ref_span or span > 1.3 * ref_span:
                            lap.is_valid = False
            else:
                for l_num in unique_laps:
                    indices = np.where(lap_arr == l_num)[0]
                    if len(indices) < 5:
                        continue
                    lap_telemetry = {k: [telemetry[k][idx] for idx in indices] for k in telemetry}
                    t0 = lap_telemetry["time"][0]
                    lap_telemetry["time"] = [v - t0 for v in lap_telemetry["time"]]
                    lap_telemetry, is_incomplete = truncate_teleport_jumps(lap_telemetry)
                    assert lap_telemetry is not None
                    if track_position:
                        lap_telemetry["distance"] = without_boundary_wrap(lap_telemetry["distance"])
                    else:
                        d0 = lap_telemetry["distance"][0]
                        lap_telemetry["distance"] = [v - d0 for v in lap_telemetry["distance"]]
                    if lap_telemetry.get("distance") and lap_telemetry["distance"][0] < 0:
                        d0 = lap_telemetry["distance"][0]
                        lap_telemetry["distance"] = [v - d0 for v in lap_telemetry["distance"]]

                    # If recorded track distance has an outlap-corrupted scale (> 1.4x physical integrated distance)
                    if "speed" in lap_telemetry and "time" in lap_telemetry and len(lap_telemetry["speed"]) > 10:
                        v_arr = np.asarray(lap_telemetry["speed"]) / 3.6
                        t_arr_phys = np.asarray(lap_telemetry["time"])
                        phys_span = float(np.sum(v_arr[1:] * np.diff(t_arr_phys)))
                        rec_span = float(np.ptp(lap_telemetry["distance"])) if lap_telemetry.get("distance") else 0.0
                        if (
                            phys_span > 500.0
                            and (rec_span > 1.4 * phys_span or rec_span < 0.6 * phys_span)
                            and rec_span > 0
                        ):
                            scale = phys_span / rec_span
                            lap_telemetry["distance"] = [v * scale for v in lap_telemetry["distance"]]
                    t_arr = lap_telemetry["time"]
                    lap_time = float(t_arr[-1] - t_arr[0]) if len(t_arr) > 1 else 0.0
                    # Hybrid lap_valid rule: the recorder's flag is unreliable when it
                    # marks most samples invalid (practice/qualifying), so only trust it
                    # when at least half the samples are valid — then a single invalid
                    # sample invalidates the whole lap. The distance filter below still
                    # runs as a final arbiter for partial laps.
                    is_valid = not is_incomplete
                    if is_valid and lap_valid is not None:
                        lap_valid_samples = [lap_valid[idx] for idx in indices[: len(t_arr)]]
                        valid_fraction = sum(1 for v in lap_valid_samples if v >= 1) / max(len(lap_valid_samples), 1)
                        if valid_fraction >= 0.5:
                            is_valid = all(v >= 1 for v in lap_valid_samples)
                    laps.append(
                        Lap(
                            lap_number=int(l_num),
                            lap_time=lap_time,
                            is_valid=is_valid,
                            is_incomplete=is_incomplete,
                            telemetry=lap_telemetry,
                        )
                    )
                # Always filter partial laps by distance covered, regardless of whether
                # we have a real track-position channel or a dead-reckoned distance.
                # A lap is invalid when it covers less than 70% or more than 130% of
                # the median span (out-laps, in-laps, and pit-inclusive laps).
                if len(laps) > 1:
                    spans = [distance_covered(lap.telemetry or {}) for lap in laps]
                    ref_span = float(np.median(spans))
                    for lap, span in zip(laps, spans, strict=True):
                        if span < 0.7 * ref_span or span > 1.3 * ref_span:
                            lap.is_valid = False

        if corrupted_laps:
            dist_series = telemetry.get("distance", [])
            total_dist = dist_series[-1] if len(dist_series) > 0 else 0.0

            # Determine circuit length generically without hardcoding track names:
            # 1. From CSV metadata header (if recorder recorded track_length_m)
            track_len = float(metadata.get("track_length_m", 0.0) or 0.0)

            # 2. From official session result lap time (best_lap_s) and telemetry flying speed
            if track_len < 100.0:
                best_lap_s = 0.0
                if is_acevo and acevo_info:
                    best_lap_s = float(acevo_info.get("best_lap") or acevo_info.get("best_lap_s") or 0.0)
                elif is_lmu and lmu_info:
                    best_lap_s = float(lmu_info.get("best_lap") or 0.0)

                if best_lap_s > 20.0 and "speed" in telemetry and "time" in telemetry:
                    flying_speeds = [
                        s for s, t in zip(telemetry["speed"], telemetry["time"], strict=False) if s > 60.0 and t > 60.0
                    ]
                    if flying_speeds:
                        avg_speed_ms = (sum(flying_speeds) / len(flying_speeds)) / 3.6
                        track_len = round(avg_speed_ms * best_lap_s, 1)

            # 3. Fallback generic default estimate if no session result lap time exists
            if track_len < 100.0:
                track_len = 4000.0

            if total_dist >= track_len * 0.7:
                lap_start_idx = 0
                current_target = track_len
                lap_idx = 1
                for i, d in enumerate(dist_series):
                    is_last_sample = i == len(dist_series) - 1
                    if d >= current_target or is_last_sample:
                        if i - lap_start_idx >= 30:
                            lap_tel = {k: telemetry[k][lap_start_idx:i] for k in telemetry}
                            t0 = lap_tel["time"][0]
                            lap_tel["time"] = [v - t0 for v in lap_tel["time"]]
                            d0 = lap_tel["distance"][0]
                            lap_tel["distance"] = [v - d0 for v in lap_tel["distance"]]
                            lap_time = float(lap_tel["time"][-1] - lap_tel["time"][0])

                            span = d - dist_series[lap_start_idx]
                            is_inc = span < 0.7 * track_len or (is_last_sample and d < current_target)
                            is_out = (lap_idx == 1 and lap_time > 120.0) or is_inc
                            is_val = not is_out and not is_inc and (0.7 * track_len <= span <= 1.3 * track_len)

                            splits = []
                            if is_val:
                                if is_acevo and acevo_info:
                                    splits = acevo_info.get("splits", [])
                                elif is_lmu and lmu_info:
                                    splits = lmu_info.get("splits", [])

                            laps.append(
                                Lap(
                                    lap_number=lap_idx,
                                    lap_time=round(lap_time, 3),
                                    sector_times=splits,
                                    is_valid=is_val,
                                    is_incomplete=is_inc,
                                    is_outlap=is_out,
                                    telemetry=lap_tel,
                                    has_telemetry=True,
                                )
                            )
                            lap_idx += 1
                        lap_start_idx = i
                        current_target += track_len

        # For AC EVO and LMU: calibrate flying lap if official session result best lap is available
        res_info = acevo_info if (is_acevo and acevo_info) else (lmu_info if (is_lmu and lmu_info) else {})
        if res_info:
            official_best = float(res_info.get("best_lap") or 0.0)
            splits = res_info.get("splits") or []
            if official_best > 20.0 and laps:
                valid_laps = [l for l in laps if l.is_valid and not l.is_outlap and not l.is_incomplete]
                if not valid_laps:
                    valid_laps = [l for l in laps if not l.is_outlap and not l.is_incomplete]
                if valid_laps:
                    flying_lap = min(valid_laps, key=lambda l: abs(l.lap_time - official_best))
                    if abs(flying_lap.lap_time - official_best) <= max(10.0, official_best * 0.10):
                        flying_lap.lap_time = round(official_best, 3)
                        flying_lap.is_valid = True
                        if splits:
                            flying_lap.sector_times = splits

        # If no explicit laps found, treat whole file as lap 1
        if not laps and n_samples > 0:
            lap_tel, is_inc = truncate_teleport_jumps(telemetry)
            assert lap_tel is not None
            total_time = float(lap_tel["time"][-1] - lap_tel["time"][0]) if len(lap_tel["time"]) > 1 else 0.0
            laps.append(
                Lap(
                    lap_number=1,
                    lap_time=total_time,
                    is_valid=not is_inc,
                    is_incomplete=is_inc,
                    telemetry=lap_tel,
                )
            )

        filename = os.path.basename(self.filepath)
        session_id = os.path.splitext(filename)[0]

        linked_file = None
        if is_acevo and acevo_info:
            linked_file = acevo_info.get("result_file")
        elif is_lmu and lmu_info:
            linked_file = lmu_info.get("result_file")

        return Session(
            session_id=session_id,
            car=metadata.get("car", self.car),
            track=metadata.get("track", self.track),
            date=session_date,
            telemetry=telemetry,
            laps=laps,
            source_file=self.filepath,
            simulator=metadata.get("simulator"),
            linked_result_file=linked_file,
        )

    def _read_metadata(self) -> dict[str, str]:
        metadata: dict[str, str] = {}
        with open(self.filepath, encoding="utf-8") as f:
            for line in f:
                if not line.startswith("#"):
                    break
                key, sep, value = line[1:].partition(":")
                if sep:
                    metadata[key.strip()] = value.strip()
        return metadata
