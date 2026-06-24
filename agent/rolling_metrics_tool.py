"""
Read-only tool for querying context-engine rolling metrics.
"""

import json
import sqlite3
from pathlib import Path
from typing import Any, Iterable, List, Dict


class RollingMetricsTool:
    """Small read-only SQLite tool exposed to the agent."""

    def __init__(self, db_path: str):
        self.db_path = Path(db_path)

    def query(self, sql: str, params: Iterable[Any] = (), limit: int = 100) -> List[Dict[str, Any]]:
        """Run a bounded read-only SELECT query against the rolling metrics DB."""
        if not self.db_path.exists():
            raise FileNotFoundError(f"rolling metrics DB not found: {self.db_path}")

        statement = sql.strip()
        if not statement.lower().startswith("select"):
            raise ValueError("only SELECT queries are allowed")
        if ";" in statement.rstrip(";"):
            raise ValueError("only one SQL statement is allowed")

        bounded_sql = statement.rstrip(";")
        if " limit " not in bounded_sql.lower():
            bounded_sql = f"{bounded_sql} LIMIT ?"
            params = tuple(params) + (limit,)

        conn = sqlite3.connect(f"file:{self.db_path}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        try:
            rows = conn.execute(bounded_sql, tuple(params)).fetchall()
            return [dict(row) for row in rows]
        finally:
            conn.close()

    def recent_windows(self, namespace: str = "", pod: str = "", limit: int = 20) -> List[Dict[str, Any]]:
        """Return recent rolling metric windows, optionally filtered to a pod."""
        filters = []
        params: list[Any] = []
        if namespace:
            filters.append("namespace = ?")
            params.append(namespace)
        if pod:
            filters.append("pod_name = ?")
            params.append(pod)

        where = f"WHERE {' AND '.join(filters)}" if filters else ""
        rows = self.query(
            f"""
            SELECT generated_at, window_start, window_end, identity, namespace, pod_name,
                   container_name, data_points, metrics_json
            FROM rolling_metric_windows
            {where}
            ORDER BY window_end DESC, identity ASC
            """,
            params,
            limit=limit,
        )
        for row in rows:
            row["metrics"] = json.loads(row.pop("metrics_json"))
        return rows

    def recent_findings(self, limit: int = 20) -> List[Dict[str, Any]]:
        """Return recent rule findings persisted by context-engine."""
        rows = self.query(
            """
            SELECT generated_at, window_start, window_end, rule_id, category,
                   severity, confidence, finding_json
            FROM rule_findings
            ORDER BY generated_at DESC
            """,
            limit=limit,
        )
        for row in rows:
            row["finding"] = json.loads(row.pop("finding_json"))
        return rows
