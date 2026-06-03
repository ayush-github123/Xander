"""
Validation layer for context schema and data quality checks.

Ensures incoming context conforms to expected structure before analysis.
"""

from typing import Tuple, List, Dict, Any, Optional


def validate_global_context(context: Dict[str, Any]) -> Tuple[bool, List[str]]:
    """
    Validate that context matches expected GlobalContext schema.
    
    Returns:
        (is_valid, list_of_errors)
    """
    errors = []
    
    # Top-level required fields
    required_top_level = ["timestamp", "containers", "cluster_stats", "system_wide_trends", "total_containers"]
    for field in required_top_level:
        if field not in context:
            errors.append(f"Missing top-level field: {field}")
    
    # Validate timestamp format (should be ISO8601)
    if "timestamp" in context:
        timestamp = context["timestamp"]
        if not isinstance(timestamp, str) or "T" not in timestamp:
            errors.append(f"Invalid timestamp format: {timestamp}")
    
    # Validate containers structure
    if "containers" in context:
        if not isinstance(context["containers"], dict):
            errors.append("'containers' must be a dictionary")
        else:
            for container_id, container_data in context["containers"].items():
                container_errors = _validate_container_context(container_id, container_data)
                errors.extend(container_errors)
    
    # Validate cluster_stats
    if "cluster_stats" in context:
        cluster_stats = context["cluster_stats"]
        if not isinstance(cluster_stats, dict):
            errors.append("'cluster_stats' must be a dictionary")
        else:
            cluster_errors = _validate_cluster_stats(cluster_stats)
            errors.extend(cluster_errors)
    
    # Validate system_wide_trends
    if "system_wide_trends" in context:
        trends = context["system_wide_trends"]
        if not isinstance(trends, dict):
            errors.append("'system_wide_trends' must be a dictionary")
    
    # Check metadata consistency
    if "containers" in context and "total_containers" in context:
        actual_count = len(context["containers"])
        stated_count = context.get("total_containers", 0)
        if actual_count != stated_count:
            errors.append(
                f"Container count mismatch: {actual_count} containers but total_containers={stated_count}"
            )
    
    # Check anomaly count consistency
    total_anomalies = 0
    for container in context.get("containers", {}).values():
        anomalies = container.get("anomalies", [])
        if isinstance(anomalies, list):
            total_anomalies += len(anomalies)
    
    stated_anomalies = context.get("cluster_stats", {}).get("total_anomalies", 0)
    if total_anomalies != stated_anomalies:
        errors.append(
            f"Anomaly count mismatch: {total_anomalies} anomalies found but total_anomalies={stated_anomalies}"
        )
    
    return len(errors) == 0, errors


def _validate_container_context(container_id: str, container_data: Dict[str, Any]) -> List[str]:
    """Validate individual container context structure."""
    errors = []
    
    if not isinstance(container_data, dict):
        return [f"Container {container_id}: data must be a dictionary"]
    
    # Required container fields
    required_fields = ["identity", "risk_level", "utilization"]
    for field in required_fields:
        if field not in container_data:
            errors.append(f"Container {container_id}: missing field '{field}'")
    
    # Validate risk_level
    if "risk_level" in container_data:
        risk_level = container_data["risk_level"]
        valid_levels = ["critical", "high", "medium", "low"]
        if risk_level not in valid_levels:
            errors.append(f"Container {container_id}: invalid risk_level '{risk_level}'")
    
    # Validate utilization (should have numeric values)
    if "utilization" in container_data:
        utilization = container_data["utilization"]
        if not isinstance(utilization, dict):
            errors.append(f"Container {container_id}: utilization must be a dictionary")
        else:
            for metric, value in utilization.items():
                if not isinstance(value, (int, float)):
                    errors.append(f"Container {container_id}: utilization.{metric} must be numeric, got {type(value)}")
    
    # Validate anomalies structure if present
    if "anomalies" in container_data:
        anomalies = container_data["anomalies"]
        if not isinstance(anomalies, list):
            errors.append(f"Container {container_id}: anomalies must be a list")
        else:
            for idx, anomaly in enumerate(anomalies):
                if not isinstance(anomaly, dict):
                    errors.append(f"Container {container_id}: anomaly[{idx}] must be a dictionary")
                else:
                    if "metric" not in anomaly or "severity" not in anomaly:
                        errors.append(
                            f"Container {container_id}: anomaly[{idx}] missing 'metric' or 'severity'"
                        )
    
    return errors


def _validate_cluster_stats(cluster_stats: Dict[str, Any]) -> List[str]:
    """Validate cluster statistics structure."""
    errors = []
    
    # Optional but if present, should be lists
    list_fields = ["top_cpu_consumers", "top_memory_growth", "most_unstable_workloads", "containers_with_errors"]
    for field in list_fields:
        if field in cluster_stats:
            value = cluster_stats[field]
            if not isinstance(value, list):
                errors.append(f"cluster_stats.{field} must be a list, got {type(value)}")
    
    # Count fields should be numeric
    count_fields = ["total_anomalies", "critical_count", "warning_count"]
    for field in count_fields:
        if field in cluster_stats:
            value = cluster_stats[field]
            if not isinstance(value, (int, float)):
                errors.append(f"cluster_stats.{field} must be numeric, got {type(value)}")
    
    return errors


def detect_context_mode(context: Dict[str, Any]) -> str:
    """
    Detect which context mode was used: 'full', 'lightweight', or 'compact'.
    
    Returns: 'full', 'lightweight', 'compact', or 'unknown'
    """
    # Compact has minimal containers and only critical/high-risk
    if len(context.get("containers", {})) < 5 and context.get("containers_at_risk", -1) < 5:
        containers_at_risk = 0
        for c in context.get("containers", {}).values():
            if c.get("risk_level") in ("critical", "high"):
                containers_at_risk += 1
        if containers_at_risk == len(context["containers"]):
            return "compact"
    
    # Lightweight has semantic-only info (no raw metrics like moving_avg, slope, etc.)
    first_container = next(iter(context.get("containers", {}).values()), None)
    if first_container:
        utilization = first_container.get("utilization", {})
        # Full mode includes aggregate metrics (avg, min, max, p95, moving_avg, slope)
        has_aggregate_metrics = any(
            key in str(utilization).lower()
            for key in ["moving_avg", "slope", "_min", "_max", "_p95", "_avg"]
        )
        if not has_aggregate_metrics:
            return "lightweight"
    
    return "full"


def count_distinct_incidents(containers: Dict[str, Dict[str, Any]]) -> int:
    """
    Count distinct incidents by deduplicating anomalies within same container
    and across containers with shared affected metrics.
    
    Returns estimated count of distinct incidents (not just total anomalies).
    """
    seen_incidents = set()
    distinct_count = 0
    
    for container_id, container_data in containers.items():
        anomalies = container_data.get("anomalies", [])
        
        for anomaly in anomalies:
            metric = anomaly.get("metric", "unknown")
            severity = anomaly.get("severity", "unknown")
            # Use metric + severity as rough deduplication key
            # (same metric + severity in same container = same incident)
            incident_key = f"{container_id}:{metric}:{severity}"
            
            if incident_key not in seen_incidents:
                seen_incidents.add(incident_key)
                distinct_count += 1
    
    return distinct_count


def validate_container_identity(container: Dict[str, Any]) -> Tuple[bool, Optional[str]]:
    """
    Validate that container identity is properly formatted.
    
    Returns: (is_valid, error_message_or_none)
    """
    identity = container.get("identity")
    if not identity:
        return False, "Missing 'identity' field"
    
    if not isinstance(identity, str):
        return False, f"'identity' must be string, got {type(identity)}"
    
    # Should roughly match format: namespace/pod/container
    parts = identity.split("/")
    if len(parts) < 2:
        return False, f"'identity' should have format 'namespace/pod/container', got: {identity}"
    
    return True, None
