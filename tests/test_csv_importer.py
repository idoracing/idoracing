import os
import tempfile

import pytest

from importers.csv_importer import CSVImporter


def test_csv_importer_valid():
    with tempfile.TemporaryDirectory() as tmpdir:
        csv_path = os.path.join(tmpdir, "telemetry.csv")
        content = """Time,Distance,Speed,Throttle,Brake,Gear,LatAccel,LongAccel,Lap
0.0,0.0,100.0,0.5,0.0,2,0.5,0.2,1
0.5,13.8,110.0,0.8,0.0,3,0.4,0.4,1
1.0,29.1,120.0,1.0,0.0,3,0.3,0.5,1
"""
        with open(csv_path, "w") as f:
            f.write(content)

        importer = CSVImporter(csv_path)
        sess = importer.parse()

        assert sess.session_id == "telemetry"
        assert len(sess.telemetry["speed"]) == 3
        assert len(sess.laps) == 1
        assert sess.laps[0].lap_number == 1


def test_csv_importer_missing_file():
    with pytest.raises(FileNotFoundError):
        CSVImporter("nonexistent_telemetry.csv")


def test_csv_laps_start_at_zero_time_and_distance(tmp_path):
    rows = ["time,distance,speed,lap"]
    rows += [f"{i * 0.1:.1f},{i * 4.0:.1f},144,1" for i in range(30)]
    rows += [f"{i * 0.1:.1f},{i * 4.0:.1f},144,2" for i in range(30, 60)]
    csv_path = tmp_path / "two_laps.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()

    assert [lap.lap_number for lap in sess.laps] == [1, 2]
    assert sess.laps[1].telemetry["time"][0] == 0.0
    assert sess.laps[1].telemetry["distance"][0] == 0.0
    assert sess.laps[1].lap_time == pytest.approx(2.9)


def test_csv_importer_setup_and_pressure_columns(tmp_path):
    rows = [
        "time,lap,lap_distance,speed,throttle,brake,gear,steer_angle,rpm,lat_accel,long_accel,yaw_rate,x,y,in_pit,lap_valid,pressure_fl,pressure_fr,pressure_rl,pressure_rr,tc,abs,brake_bias"
    ]
    for i in range(25):
        rows.append(
            f"{i * 0.1:.3f},1,{i * 10:.2f},180.0,1.0,0.0,4,0.0,7500,0.5,0.2,0.0,10.0,20.0,0,1,26.8,26.9,26.7,26.8,3.0,4.0,54.5"
        )
    csv_path = tmp_path / "acevo_setup_tel.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()
    assert "pressure_fl" in sess.telemetry
    assert "tc" in sess.telemetry
    assert "abs" in sess.telemetry
    assert "brake_bias" in sess.telemetry
    assert sess.telemetry["pressure_fl"][0] == 26.8
    assert sess.telemetry["tc"][0] == 3.0
    assert sess.telemetry["abs"][0] == 4.0
    assert sess.telemetry["brake_bias"][0] == 54.5


def test_csv_importer_stuck_lap_counter_segments_by_distance_wrap(tmp_path):
    """ACC practice/qualifying can leave CompletedLaps stuck at 0, so the recorder writes
    lap=1 for every sample even after 10 real laps. The importer must detect this and
    segment by lap_distance wraps (position dropping from ~track_len back to ~0)."""
    track_len = 200.0
    samples_per_lap = 100  # 2s at 50Hz per lap
    n_real_laps = 10
    speed_ms = 100.0  # 360 km/h — physically consistent with covering track_len in 2s
    rows = [
        "# simulator: Assetto Corsa Competizione",
        "# track: Oslo",
        "# car: Porsche 911 GT3 R (992)",
        f"# track_length_m: {track_len}",
        "time,lap,lap_distance,speed,throttle,brake,gear,lat_accel,long_accel,lap_valid",
    ]
    t = 0.0
    for _lap in range(n_real_laps):
        for i in range(samples_per_lap):
            # lap_distance goes 0 -> track_len within each lap, then wraps back to 0
            lap_dist = (i / samples_per_lap) * track_len
            speed_kmh = speed_ms * 3.6
            rows.append(f"{t:.3f},1,{lap_dist:.2f},{speed_kmh:.2f},1.0,0.0,4,0.5,0.2,1")
            t += 0.02  # 50Hz
    csv_path = tmp_path / "acc_oslo_stuck_lap.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()

    # Must recover all 10 real laps despite lap column being stuck at 1
    assert len(sess.laps) == n_real_laps
    assert [lap.lap_number for lap in sess.laps] == list(range(1, n_real_laps + 1))
    # Each lap should span roughly the full track length
    for lap in sess.laps:
        span = max(lap.telemetry["distance"]) - min(lap.telemetry["distance"])
        assert span == pytest.approx(track_len, rel=0.05)


def test_csv_importer_normal_laps_not_affected_by_stuck_detection(tmp_path):
    """When the lap counter works normally (unique laps match physical distance), the
    stuck-lap detection must NOT trigger and laps must be segmented by the lap column."""
    rows = ["time,lap,lap_distance,speed,throttle,brake,gear,lat_accel,long_accel,lap_valid"]
    t = 0.0
    for lap_num in (1, 2):
        for i in range(50):
            lap_dist = (i / 50) * 3000.0
            rows.append(f"{t:.3f},{lap_num},{lap_dist:.2f},180.0,1.0,0.0,4,0.5,0.2,1")
            t += 0.02
    csv_path = tmp_path / "normal_laps.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()

    assert [lap.lap_number for lap in sess.laps] == [1, 2]


def test_csv_importer_stuck_lap_counter_accumulated_distance(tmp_path):
    """When track_length_m=0 in metadata and the track is not one we have mapped,
    lap_distance is accumulated (not track position). The importer must estimate
    track length from distance drops and segment at the tracker-reset drops."""
    samples_per_lap = 100  # 2s at 50Hz
    n_real_laps = 8
    speed_ms = 250.0  # 900 km/h — covers 500m in 2s
    rows = [
        "# simulator: Assetto Corsa Competizione",
        "# track: oslo",
        "# car: porsche_992_gt3_r",
        "# track_length_m: 0.0",
        "time,lap,lap_distance,speed,throttle,brake,gear,lat_accel,long_accel,lap_valid",
    ]
    t = 0.0
    acc_dist = 0.0
    for lap_i in range(n_real_laps):
        for i in range(samples_per_lap):
            # Accumulated distance grows monotonically within a lap; at lap boundary
            # the distanceTracker resets, causing a large backward drop
            if i == 0 and lap_i > 0:
                acc_dist = 0.0  # tracker reset: big drop from ~track_len*lap_i to 0
            acc_dist += speed_ms * 0.02
            speed_kmh = speed_ms * 3.6
            rows.append(f"{t:.3f},1,{acc_dist:.2f},{speed_kmh:.2f},1.0,0.0,4,0.5,0.2,1")
            t += 0.02
    csv_path = tmp_path / "acc_monza_stuck_accumulated.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()

    # Must recover multiple laps despite lap column stuck at 1 and accumulated distance
    assert len(sess.laps) >= n_real_laps - 1  # allow off-by-one at boundaries
    assert [lap.lap_number for lap in sess.laps] == list(range(1, len(sess.laps) + 1))


def test_csv_importer_stuck_lap_counter_known_track_segments_by_official_length(tmp_path):
    """When track_length_m=0 but the track is mapped (monza = 5793 m), the importer must
    segment the physically integrated distance at multiples of the official lap length,
    even if the raw accumulated lap_distance has no reliable resets (pure monotonic)."""
    track_len = 5793.0  # official Monza length from analysis.tracks
    samples_per_lap = 100  # 2s at 50Hz per lap
    n_real_laps = 6
    speed_ms = track_len / 2.0  # covers exactly one lap in 2s
    rows = [
        "# simulator: Assetto Corsa Competizione",
        "# track: monza",
        "# car: porsche_992_gt3_r",
        "# track_length_m: 0.0",
        "time,lap,lap_distance,speed,throttle,brake,gear,lat_accel,long_accel,lap_valid",
    ]
    t = 0.0
    acc_dist = 0.0
    for _lap in range(n_real_laps):
        for _i in range(samples_per_lap):
            # Accumulated distance grows monotonically with NO drops (unreliable tracker)
            acc_dist += speed_ms * 0.02
            speed_kmh = speed_ms * 3.6
            rows.append(f"{t:.3f},1,{acc_dist:.2f},{speed_kmh:.2f},1.0,0.0,4,0.5,0.2,1")
            t += 0.02
    csv_path = tmp_path / "acc_monza_stuck_known_len.csv"
    csv_path.write_text("\n".join(rows))

    sess = CSVImporter(str(csv_path)).parse()

    # Must recover all real laps by segmenting at multiples of the official 5793 m
    assert len(sess.laps) == n_real_laps
    for lap in sess.laps:
        span = max(lap.telemetry["distance"]) - min(lap.telemetry["distance"])
        assert span == pytest.approx(track_len, rel=0.05)
