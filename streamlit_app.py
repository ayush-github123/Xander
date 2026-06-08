import sqlite3
import os
import threading
import time
import random
from datetime import datetime, timezone, timedelta
import pandas as pd
import numpy as np
import streamlit as st
import altair as alt
import requests
import json
import subprocess

# Single-file Streamlit app implementing:
# 1) telemetry collection daemon
# 2) aggregation engine (1m & 5m)
# 3) simple context engine
# 4) a rule-based agent that responds using context

COLLECTOR_DB_SNAPSHOT = "/tmp/collector-metrics.db"

# Default DB candidates to try for real telemetry databases
DEFAULT_DB_CANDIDATES = [
    COLLECTOR_DB_SNAPSHOT,
    "telemetry-collector/metrics.db",
    "./metrics.db",
    "/tmp/metrics.db",
    "telemetry.db",
]

WINDOW_LABELS = {
    60: "1 minute",
    120: "2 minutes",
    300: "5 minutes",
    900: "15 minutes",
    3600: "1 hour",
    21600: "6 hours",
    86400: "24 hours",
}

SYSTEM_NAMESPACES = {"kube-system", "kube-public", "kube-node-lease", "telemetry-system"}

SCENARIOS = {
    "Scenario 1: log-heavy noisy neighbor": ["pod-x-noisy", "pod-y-db"],
    "Scenario 2: shared PVC bottleneck": ["pod-x-writer", "pod-y-reader"],
    "Scenario 3: page cache contention": ["pod-x-cache-clearer", "pod-y-web"],
    "Scenario 4: kubelet disk pressure": ["pod-x-disk-filler", "pod-y-critical"],
}

SCENARIO_SIGNAL_CHECKS = {
    "Scenario 1: log-heavy noisy neighbor": [
        ("pod-x-noisy", "disk_write_max_mib_s", 1.0, "`pod-x-noisy` is present, but disk write throughput is low."),
        ("pod-y-db", "processes", 1.0, "`pod-y-db` is present, but database process signals are weak."),
    ],
    "Scenario 2: shared PVC bottleneck": [
        ("pod-x-writer", "disk_write_max_mib_s", 1.0, "`pod-x-writer` is present, but writer disk throughput is low."),
        ("pod-y-reader", "disk_read_max_mib_s", 0.1, "`pod-y-reader` is present, but reader disk throughput is low."),
    ],
    "Scenario 3: page cache contention": [
        ("pod-x-cache-clearer", "disk_read_max_mib_s", 1.0, "`pod-x-cache-clearer` is present, but cache-churn read throughput is low."),
        ("pod-y-web", "net_tx_max_kib_s", 1.0, "`pod-y-web` is present, but web serving traffic is low."),
    ],
    "Scenario 4: kubelet disk pressure": [
        ("pod-x-disk-filler", "processes", 1.0, "`pod-x-disk-filler` is present, but filler process signals are weak."),
        ("pod-y-critical", "processes", 1.0, "`pod-y-critical` is present, but victim process signals are weak."),
    ],
}

DB_PATH = "telemetry.db"
BYTES_PER_KIB = 1024
BYTES_PER_MIB = 1024 * 1024

def init_db(conn: sqlite3.Connection):
    cur = conn.cursor()
    cur.execute(
        """
        CREATE TABLE IF NOT EXISTS metrics (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT NOT NULL,
            pod_name TEXT,
            pod_namespace TEXT,
            container_name TEXT,
            cpu_usage REAL,
            memory_rss INTEGER,
            network_rx_bytes INTEGER,
            diskio_read_bytes INTEGER,
            seq INTEGER
        )
        """
    )
    conn.commit()


def open_db(path=DB_PATH):
    # If the DB doesn't exist, create it (for simulation mode); otherwise open existing DB.
    created = False
    if not os.path.exists(path):
        # create parent dir if needed
        parent = os.path.dirname(path)
        if parent and not os.path.exists(parent):
            try:
                os.makedirs(parent, exist_ok=True)
            except Exception:
                pass
        created = True
    conn = sqlite3.connect(path, check_same_thread=False)
    # Only initialize schema when we created the DB here (to avoid clobbering real DBs)
    if created:
        init_db(conn)
    return conn


class TelemetryCollector(threading.Thread):
    def __init__(self, conn, interval=5, pods=None):
        super().__init__(daemon=True)
        self.conn = conn
        self.interval = interval
        self._stop = threading.Event()
        self.seq = 0
        if pods is None:
            self.pods = [
                ("kube-proxy", "kube-system", "kube-proxy"),
                ("postgres", "test-scenarios", "postgres"),
                ("collector", "telemetry-system", "collector"),
                ("noisy-pod", "default", "noisy"),
            ]
        else:
            self.pods = pods

    def stop(self):
        self._stop.set()

    def stopped(self):
        return self._stop.is_set()

    def run(self):
        cur = self.conn.cursor()
        while not self._stop.is_set():
            now = datetime.now(timezone.utc).isoformat()
            for pod_name, pod_ns, container in self.pods:
                # Simulate telemetry values
                # CPU usage simulated in cores (0..2)
                cpu = max(0.0, random.gauss(0.2, 0.4))
                # memory in bytes
                memory = int(max(10e6, random.gauss(80e6, 25e6)))
                net = int(max(0, random.gauss(10000, 7000)))
                disk = int(max(0, random.gauss(2000, 1500)))
                self.seq += 1
                cur.execute(
                    "INSERT INTO metrics (timestamp, pod_name, pod_namespace, container_name, cpu_usage, memory_rss, network_rx_bytes, diskio_read_bytes, seq) VALUES (?,?,?,?,?,?,?,?,?)",
                    (now, pod_name, pod_ns, container, cpu, memory, net, disk, self.seq),
                )
            self.conn.commit()
            time.sleep(self.interval)


def fetch_metrics_df(conn, since_seconds=300, limit=50000):
    cur = conn.cursor()
    cur.execute("PRAGMA table_info(metrics)")
    cols = {row[1] for row in cur.fetchall()}

    since = (datetime.now(timezone.utc) - timedelta(seconds=since_seconds)).isoformat()

    def col_expr(name):
        return name if name in cols else f"0 AS {name}"

    cpu_expr = "cpu_usage AS cpu_total" if "cpu_usage" in cols else "COALESCE(cpu_user_time,0) + COALESCE(cpu_system_time,0) AS cpu_total"
    seq_expr = "seq" if "seq" in cols else "NULL AS seq"

    query = f"""
        SELECT
            id,
            timestamp,
            pod_name,
            pod_namespace,
            container_name,
            {cpu_expr},
            {col_expr("cpu_throttled_time")},
            {col_expr("memory_rss")},
            {col_expr("memory_working_set")},
            {col_expr("memory_limit")},
            {col_expr("network_rx_bytes")},
            {col_expr("network_tx_bytes")},
            {col_expr("network_rx_packets")},
            {col_expr("network_tx_packets")},
            {col_expr("diskio_read_bytes")},
            {col_expr("diskio_write_bytes")},
            {col_expr("diskio_read_ops")},
            {col_expr("diskio_write_ops")},
            {col_expr("process_count")},
            {seq_expr}
        FROM metrics
        WHERE datetime(timestamp) >= datetime(?)
        ORDER BY timestamp ASC
        LIMIT ?
    """
    cur.execute(query, (since, limit))
    rows = cur.fetchall()
    columns = [
        "id",
        "timestamp",
        "pod_name",
        "pod_namespace",
        "container_name",
        "cpu_total",
        "cpu_throttled_time",
        "memory_rss",
        "memory_working_set",
        "memory_limit",
        "network_rx_bytes",
        "network_tx_bytes",
        "network_rx_packets",
        "network_tx_packets",
        "diskio_read_bytes",
        "diskio_write_bytes",
        "diskio_read_ops",
        "diskio_write_ops",
        "process_count",
        "seq",
    ]
    if not rows:
        return pd.DataFrame(columns=columns)

    df = pd.DataFrame(rows, columns=columns)
    df["timestamp"] = pd.to_datetime(df["timestamp"])
    return add_readable_metrics(df)


def add_readable_metrics(df):
    readable_cols = [
        "cpu_cores",
        "memory_mib",
        "memory_working_set_mib",
        "memory_limit_mib",
        "disk_read_mib_s",
        "disk_write_mib_s",
        "net_rx_kib_s",
        "net_tx_kib_s",
    ]
    if df.empty:
        return df.assign(**{col: pd.Series(dtype="float64") for col in readable_cols})

    df = df.sort_values(["pod_namespace", "pod_name", "container_name", "timestamp"]).copy()
    group_cols = ["pod_namespace", "pod_name", "container_name"]
    elapsed = df.groupby(group_cols)["timestamp"].diff().dt.total_seconds()
    elapsed = elapsed.where(elapsed > 0)

    if df["cpu_total"].max() > 1_000_000:
        cpu_delta = df.groupby(group_cols)["cpu_total"].diff()
        df["cpu_cores"] = (cpu_delta / 1_000_000_000 / elapsed).clip(lower=0).fillna(0)
    else:
        df["cpu_cores"] = df["cpu_total"]

    df["memory_mib"] = df["memory_rss"] / BYTES_PER_MIB
    df["memory_working_set_mib"] = df["memory_working_set"] / BYTES_PER_MIB
    df["memory_limit_mib"] = df["memory_limit"] / BYTES_PER_MIB

    def counter_rate(source, divisor):
        delta = df.groupby(group_cols)[source].diff()
        return (delta / elapsed / divisor).clip(lower=0).fillna(0)

    df["disk_read_mib_s"] = counter_rate("diskio_read_bytes", BYTES_PER_MIB)
    df["disk_write_mib_s"] = counter_rate("diskio_write_bytes", BYTES_PER_MIB)
    df["net_rx_kib_s"] = counter_rate("network_rx_bytes", BYTES_PER_KIB)
    df["net_tx_kib_s"] = counter_rate("network_tx_bytes", BYTES_PER_KIB)
    return df


def get_db_status(conn):
    cur = conn.cursor()
    cur.execute("SELECT COUNT(*), MAX(timestamp) FROM metrics")
    row_count, latest = cur.fetchone()
    latest_dt = pd.to_datetime(latest, utc=True) if latest else None
    return row_count or 0, latest, latest_dt


def db_latest_time(path):
    if not os.path.exists(path):
        return None
    try:
        conn = sqlite3.connect(path)
        _, _, latest_dt = get_db_status(conn)
        conn.close()
        return latest_dt
    except sqlite3.Error:
        return None


def db_age_seconds(path):
    latest_dt = db_latest_time(path)
    if latest_dt is None:
        return None
    return (pd.Timestamp.now(tz="UTC") - latest_dt).total_seconds()


def freshest_db(candidates):
    existing = [p for p in candidates if os.path.exists(p)]
    if not existing:
        return candidates[0]
    return max(existing, key=lambda p: db_latest_time(p) or pd.Timestamp.min.tz_localize("UTC"))


def sync_collector_db(path="/tmp/collector-metrics.db"):
    script_path = os.path.join(os.path.dirname(__file__), "scripts", "sync-collector-db.sh")
    if os.path.exists(script_path):
        subprocess.check_call(["bash", script_path, path], timeout=60)
        return path

    pod_cmd = ["kubectl", "get", "pod", "-n", "telemetry-system", "-l", "app=telemetry-collector", "-o", "jsonpath={.items[0].metadata.name}"]
    pod = subprocess.check_output(pod_cmd, text=True, timeout=10).strip()
    subprocess.check_call(["kubectl", "cp", f"telemetry-system/{pod}:/tmp/metrics.db", path], timeout=20)
    return path


def close_db_conn():
    conn = st.session_state.get("db_conn")
    if conn is not None:
        conn.close()
    st.session_state.pop("db_conn", None)


def ensure_db_conn(path):
    if "db_conn" not in st.session_state or st.session_state.get("db_path") != path:
        close_db_conn()
        st.session_state.db_path = path
        st.session_state.db_conn = open_db(path)
    return st.session_state.db_conn


def refresh_collector_snapshot(path, interval):
    if path != COLLECTOR_DB_SNAPSHOT:
        return
    snapshot_age = db_age_seconds(path)
    if snapshot_age is not None and snapshot_age <= max(interval, 2):
        return
    close_db_conn()
    sync_collector_db(path)


def filter_system_pods(df, show_system_pods):
    if show_system_pods or df.empty or "pod_namespace" not in df.columns:
        return df
    filtered = df[~df["pod_namespace"].isin(SYSTEM_NAMESPACES)].copy()
    return filtered if not filtered.empty else df


def scenario_signal_summary(df, expected_pods):
    if df.empty:
        return pd.DataFrame()
    scenario_df = df[df["pod_name"].isin(expected_pods)].copy()
    if scenario_df.empty:
        return pd.DataFrame()

    summary = scenario_df.groupby("pod_name").agg(
        samples=("id", "count"),
        latest_sample=("timestamp", "max"),
        cpu_max=("cpu_cores", "max"),
        disk_read_max_mib_s=("disk_read_mib_s", "max"),
        disk_write_max_mib_s=("disk_write_mib_s", "max"),
        disk_write_mean_mib_s=("disk_write_mib_s", "mean"),
        net_tx_max_kib_s=("net_tx_kib_s", "max"),
        processes=("process_count", "max"),
    ).reset_index()

    return summary.round({
        "cpu_max": 3,
        "disk_read_max_mib_s": 3,
        "disk_write_max_mib_s": 3,
        "disk_write_mean_mib_s": 3,
        "net_tx_max_kib_s": 2,
        "processes": 1,
    })


def render_scenario_check(df, scenario_name, expected_pods):
    st.markdown(f"**{scenario_name} check**")
    if df.empty:
        st.info("No recent metrics to check scenario pods yet.")
        return

    observed = set(df["pod_name"].dropna())
    missing = [pod for pod in expected_pods if pod not in observed]
    if missing:
        st.warning(
            "Missing expected scenario pod(s) in recent metrics: "
            + ", ".join(missing)
            + ". Check `kubectl get pods` and collector logs before trusting this run."
        )
    else:
        st.success("Expected scenario pods are present in recent metrics.")

    summary = scenario_signal_summary(df, expected_pods)
    if not summary.empty:
        st.dataframe(summary, hide_index=True)

        for pod_name, metric, threshold, warning in SCENARIO_SIGNAL_CHECKS.get(scenario_name, []):
            pod_summary = summary[summary["pod_name"] == pod_name]
            if not pod_summary.empty and metric in pod_summary.columns:
                value = float(pod_summary.iloc[0][metric] or 0)
                if value < threshold:
                    st.warning(
                        warning
                        + " Redeploy the scenario with the updated manifest and wait for fresh samples."
                    )

        if scenario_name == "Scenario 4: kubelet disk pressure":
            filler = summary[summary["pod_name"] == "pod-x-disk-filler"]
            if not filler.empty and float(filler.iloc[0].get("disk_write_max_mib_s", 0) or 0) < 0.1:
                st.warning(
                    "This metrics view can show the disk-filler pod, but node DiskPressure itself "
                    "must be confirmed with `kubectl describe node` while the scenario is running."
                )


def render_telemetry_dashboard(db_path, window_seconds, interval, running, scenario_name, expected_pods, show_system_pods):
    if running:
        try:
            refresh_collector_snapshot(db_path, interval)
        except Exception as e:
            st.warning(f"Live refresh failed: {e}")

    conn = ensure_db_conn(db_path)
    row_count, latest_raw, latest_dt = get_db_status(conn)
    df = fetch_metrics_df(conn, since_seconds=window_seconds)
    visible_df = filter_system_pods(df, show_system_pods)

    col1, col2 = st.columns([2, 1])

    with col1:
        st.subheader("Telemetry charts")
        render_scenario_check(df, scenario_name, expected_pods)
        pods = ["All"] + sorted(visible_df["pod_name"].dropna().unique().tolist()) if not visible_df.empty else ["All"]
        pod_sel = st.selectbox("Pod to view", pods)
        plot_df = visible_df.copy()
        if pod_sel != "All":
            plot_df = plot_df[plot_df["pod_name"] == pod_sel]

        if latest_dt is not None:
            age_seconds = (pd.Timestamp.now(tz="UTC") - latest_dt).total_seconds()
            st.caption(f"Rows: {row_count} | Latest sample: {latest_raw} | Age: {age_seconds:.1f}s")

        if plot_df.empty:
            if row_count:
                st.info("No telemetry in the selected window. The database has older samples, but no fresh rows for this range.")
            else:
                st.info("No telemetry yet — collector is populating the database.")
        else:
            st.markdown("**CPU usage (cores)**")
            cpu_chart = alt.Chart(plot_df).mark_line(point=True).encode(
                x=alt.X("timestamp:T", title="time"),
                y=alt.Y("cpu_cores:Q", title="cores", axis=alt.Axis(format=".2f")),
                color="pod_name:N",
                tooltip=["timestamp", "pod_name", alt.Tooltip("cpu_cores:Q", title="cores", format=".3f")],
            ).interactive()
            st.altair_chart(cpu_chart, use_container_width=True)

            st.markdown("**Memory (MiB)**")
            mem_chart = alt.Chart(plot_df).mark_line().encode(
                x="timestamp:T",
                y=alt.Y("memory_mib:Q", title="MiB", axis=alt.Axis(format=".1f")),
                color="pod_name:N",
                tooltip=["timestamp", "pod_name", alt.Tooltip("memory_mib:Q", title="MiB", format=".1f")],
            ).interactive()
            st.altair_chart(mem_chart, use_container_width=True)

            st.markdown("**Disk I/O (MiB/s)**")
            disk_df = plot_df.melt(
                id_vars=["timestamp", "pod_name"],
                value_vars=["disk_read_mib_s", "disk_write_mib_s"],
                var_name="direction",
                value_name="mib_s",
            )
            disk_chart = alt.Chart(disk_df).mark_line().encode(
                x=alt.X("timestamp:T", title="time"),
                y=alt.Y("mib_s:Q", title="MiB/s", axis=alt.Axis(format=".2f")),
                color="pod_name:N",
                strokeDash="direction:N",
                tooltip=["timestamp", "pod_name", "direction", alt.Tooltip("mib_s:Q", title="MiB/s", format=".3f")],
            ).interactive()
            st.altair_chart(disk_chart, use_container_width=True)

            st.markdown("**Network I/O (KiB/s)**")
            net_df = plot_df.melt(
                id_vars=["timestamp", "pod_name"],
                value_vars=["net_rx_kib_s", "net_tx_kib_s"],
                var_name="direction",
                value_name="kib_s",
            )
            net_chart = alt.Chart(net_df).mark_line().encode(
                x=alt.X("timestamp:T", title="time"),
                y=alt.Y("kib_s:Q", title="KiB/s", axis=alt.Axis(format=".2f")),
                color="pod_name:N",
                strokeDash="direction:N",
                tooltip=["timestamp", "pod_name", "direction", alt.Tooltip("kib_s:Q", title="KiB/s", format=".2f")],
            ).interactive()
            st.altair_chart(net_chart, use_container_width=True)

            st.markdown("**Processes**")
            proc_chart = alt.Chart(plot_df).mark_line(point=True).encode(
                x=alt.X("timestamp:T", title="time"),
                y=alt.Y("process_count:Q", title="processes"),
                color="pod_name:N",
                tooltip=["timestamp", "pod_name", "process_count"],
            ).interactive()
            st.altair_chart(proc_chart, use_container_width=True)

    with col2:
        st.subheader("Aggregation")
        df_1m = filter_system_pods(fetch_metrics_df(conn, since_seconds=60), show_system_pods)
        df_5m = filter_system_pods(fetch_metrics_df(conn, since_seconds=300), show_system_pods)
        agg_1m = aggregate(df_1m)
        agg_5m = aggregate(df_5m)
        st.markdown("**1 minute aggregates**")
        st.dataframe(agg_1m)
        st.markdown("**5 minute aggregates**")
        st.dataframe(agg_5m)

        st.subheader("Context")
        ctx = context_engine(agg_1m, agg_5m)
        st.write(ctx.get("summary"))
        if ctx.get("issues"):
            st.write(pd.DataFrame(ctx.get("issues")))

        st.subheader("Agent")
        user_q = st.text_input(
            "Ask the agent about the cluster (e.g. 'issues' / 'summary' / 'explain postgres')",
            key="agent_query",
        )
        if st.button("Get response", key="agent_submit"):
            st.session_state.agent_response = agent_response(user_q, ctx)
        if st.session_state.get("agent_response"):
            st.code(st.session_state.agent_response)

        st.markdown("---")
        st.subheader("Telemetry API (Go service)")
        api_url = st.session_state.get("api_url", "http://127.0.0.1:8081")
        st.write(f"API: {api_url}")
        if st.button("Refresh from API"):
            try:
                r = requests.get(api_url + "/top-risk?limit=10&window=60", timeout=5)
                top = r.json()
            except Exception as e:
                st.error(f"API request failed: {e}")
                top = None
            try:
                r2 = requests.get(api_url + "/incidents", timeout=5)
                inc = r2.json()
            except Exception:
                inc = None
            try:
                r3 = requests.get(api_url + "/cluster-summary", timeout=5)
                summ = r3.json()
            except Exception:
                summ = None
            st.markdown("**Top risk**")
            if top:
                st.write(pd.DataFrame(top))
            else:
                st.write("No data from /top-risk")
            st.markdown("**Incidents**")
            if inc:
                st.write(inc)
            else:
                st.write("No data from /incidents")
            st.markdown("**Cluster summary**")
            if summ:
                st.write(summ)
            else:
                st.write("No data from /cluster-summary")


def aggregate(df, by="pod_name"):
    if df.empty:
        return pd.DataFrame()
    grp = df.groupby(by).agg(
        cpu_mean=("cpu_cores", "mean"),
        cpu_max=("cpu_cores", "max"),
        mem_mean=("memory_mib", "mean"),
        disk_read_mib_s=("disk_read_mib_s", "mean"),
        disk_write_mib_s=("disk_write_mib_s", "mean"),
        net_rx_kib_s=("net_rx_kib_s", "mean"),
        net_tx_kib_s=("net_tx_kib_s", "mean"),
        processes=("process_count", "mean"),
        rows=("id", "count"),
    ).reset_index()
    return grp.round({
        "cpu_mean": 3,
        "cpu_max": 3,
        "mem_mean": 1,
        "disk_read_mib_s": 3,
        "disk_write_mib_s": 3,
        "net_rx_kib_s": 2,
        "net_tx_kib_s": 2,
        "processes": 1,
    })


def context_engine(agg_1m: pd.DataFrame, agg_5m: pd.DataFrame):
    """
    Build simple context from aggregates. Detect pods with high cpu or sudden CPU growth.
    """
    issues = []
    if agg_1m.empty:
        return {"issues": issues, "summary": "no data"}
    # Ensure both have a pod_name column
    if "pod_name" not in agg_1m.columns:
        agg_1m = agg_1m.reset_index().rename(columns={0: "pod_name"})
    if agg_5m is None or agg_5m.empty:
        agg_5m = pd.DataFrame(columns=["pod_name", "cpu_mean", "cpu_max", "mem_mean", "disk_read_mib_s", "disk_write_mib_s", "net_rx_kib_s", "net_tx_kib_s", "processes", "rows"]) 
    merged = pd.merge(agg_1m, agg_5m, on="pod_name", how="left", suffixes=("_1m", "_5m"))
    for _, row in merged.iterrows():
        pod = row["pod_name"]
        cpu1 = float(row.get("cpu_mean_1m", 0) or 0)
        cpu5 = float(row.get("cpu_mean_5m", 0) or 0)
        if cpu1 > 1.0:
            issues.append({"pod": pod, "symptom": "high_cpu", "cpu_1m": cpu1, "cpu_5m": cpu5})
        elif cpu5 > 0 and cpu1 > cpu5 * 2 and cpu1 > 0.5:
            issues.append({"pod": pod, "symptom": "cpu_spike", "cpu_1m": cpu1, "cpu_5m": cpu5})
    summary = f"{len(merged)} pods observed, {len(issues)} potential issues detected"
    return {"issues": issues, "summary": summary, "merged": merged}


def agent_response(user_query: str, context: dict):
    """
    Simple rule-based agent that uses context to answer queries.
    """
    q = (user_query or "").lower()
    issues = context.get("issues", [])
    merged = context.get("merged")
    if not user_query or user_query.strip() == "":
        return "Ask a question about the cluster or type 'summary' / 'issues' / 'explain <pod>'"
    if "issues" in q or "problem" in q or "what's wrong" in q or "whats wrong" in q:
        if not issues:
            return "Agent: no issues detected in the recent window."
        lines = [f"Agent detected {len(issues)} issue(s):"]
        for it in issues:
            lines.append(f"- Pod {it['pod']}: {it['symptom']}, cpu_1m={it['cpu_1m']:.2f}, cpu_5m={it['cpu_5m']:.2f}")
        return "\n".join(lines)
    if q.startswith("explain "):
        pod = q.split("explain ", 1)[1].strip()
        if merged is None or merged.empty:
            return f"No recent data to explain {pod}."
        r = merged[merged["pod_name"] == pod]
        if r.empty:
            return f"No data for pod {pod}."
        row = r.iloc[0]
        return (
            f"Pod {pod} — cpu_mean_1m={row.get('cpu_mean_1m', np.nan):.3f}, cpu_mean_5m={row.get('cpu_mean_5m', np.nan):.3f}, "
            f"mem_mean_1m={row.get('mem_mean_1m', np.nan):.0f}"
        )
    if "summary" in q or "status" in q:
        return f"Agent summary: {context.get('summary', 'no summary')}"
    # default: echo context summary + small advice
    s = context.get("summary", "no summary")
    advice = "No immediate action." if not issues else "Investigate pods listed in issues; consider scaling or debugging."
    return f"Agent: {s}\n{advice}"


def main():
    st.set_page_config(page_title="Telemetry → Aggregation → Context → Agent", layout="wide")
    st.title("Telemetry → Aggregation → Context → Agent")

    # Sidebar controls
    st.sidebar.header("Collector & Settings")
    interval = st.sidebar.slider("Collection interval (s)", min_value=1, max_value=30, value=5)
    running = st.sidebar.checkbox("Collector running", value=True)
    live_charts = st.sidebar.checkbox("Live charts", value=True)
    window_seconds = st.sidebar.selectbox(
        "Display window",
        options=list(WINDOW_LABELS),
        index=2,
        format_func=WINDOW_LABELS.get,
    )
    scenario_name = st.sidebar.selectbox("Scenario check", options=list(SCENARIOS), index=0)
    expected_pods = SCENARIOS[scenario_name]
    show_system_pods = st.sidebar.checkbox("Show system pods", value=False)
    st.sidebar.markdown("---")
    st.sidebar.header("Database")
    default_choice = freshest_db(DEFAULT_DB_CANDIDATES)
    current_path = st.session_state.get("db_path", default_choice)
    db_choice = st.sidebar.selectbox(
        "Choose metrics DB",
        options=DEFAULT_DB_CANDIDATES,
        index=DEFAULT_DB_CANDIDATES.index(current_path) if current_path in DEFAULT_DB_CANDIDATES else DEFAULT_DB_CANDIDATES.index(default_choice),
    )
    custom = st.sidebar.text_input(
        "Or enter custom DB path (leave blank to use selection)",
        value="" if current_path in DEFAULT_DB_CANDIDATES else current_path,
    )
    if st.sidebar.button("Refresh collector DB"):
        try:
            close_db_conn()
            st.session_state.db_path = sync_collector_db()
            st.rerun()
        except Exception as e:
            st.sidebar.error(f"Refresh failed: {e}")
    db_path = custom.strip() or db_choice
    ensure_db_conn(db_path)
    st.session_state.api_url = st.sidebar.text_input(
        "Telemetry API URL",
        value=st.session_state.get("api_url", "http://127.0.0.1:8081"),
    )

    # Manage collector thread lifecycle in session_state
    if "collector" not in st.session_state:
        st.session_state.collector = None

    # Start or stop collector depending on checkbox
    # If the chosen DB path exists and is likely the real collector DB, do NOT start simulation by default.
    db_exists = os.path.exists(st.session_state.db_path)
    simulate = st.sidebar.checkbox("Simulate collector if no DB found", value=False)
    if running and not db_exists and (st.session_state.collector is None or not st.session_state.collector.is_alive()) and simulate:
        st.session_state.collector = TelemetryCollector(st.session_state.db_conn, interval=interval)
        st.session_state.collector.start()
    if (not running or (db_exists and not simulate)) and st.session_state.collector is not None:
        try:
            st.session_state.collector.stop()
        except Exception:
            pass
        st.session_state.collector = None

    row_count, latest_raw, latest_dt = get_db_status(st.session_state.db_conn)
    st.sidebar.caption(f"Rows: {row_count}")
    if latest_dt is not None:
        age_seconds = (pd.Timestamp.now(tz="UTC") - latest_dt).total_seconds()
        st.sidebar.caption(f"Latest sample: {latest_raw}")
        if age_seconds > window_seconds:
            st.sidebar.warning(f"No samples in the selected {window_seconds}s window.")

    if live_charts and running:
        live_dashboard = st.fragment(run_every=f"{max(interval, 1)}s")(render_telemetry_dashboard)
        live_dashboard(db_path, window_seconds, interval, running, scenario_name, expected_pods, show_system_pods)
    else:
        render_telemetry_dashboard(db_path, window_seconds, interval, running, scenario_name, expected_pods, show_system_pods)

    st.markdown("---")
    st.caption("This single-file demo simulates telemetry collection, runs lightweight aggregation and context analysis, and exposes a small rule-based agent to answer queries using that context.")


if __name__ == "__main__":
    main()
