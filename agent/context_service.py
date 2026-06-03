"""
Context Service - Python interface for context layer
Provides APIs for the Agent to query and consume context
"""

import json
import os
from pathlib import Path
from typing import Dict, List, Optional, Any
from dataclasses import dataclass
from datetime import datetime


@dataclass
class ContainerContext:
    """Represents context for a single container"""
    identity: str
    namespace: str
    pod_name: str
    container_name: str
    risk_level: str
    health_indicators: Dict[str, str]
    anomalies: List[Dict[str, Any]]
    recommendations: List[str]
    utilization: Dict[str, float]
    cpu_usage: float
    memory_usage: float
    disk_io_activity: float
    network_busy: float
    incident_context: Dict[str, Any] = None
    
    def __post_init__(self):
        # Handle None default for mutable field
        if self.incident_context is None:
            self.incident_context = {}


@dataclass
class GlobalContext:
    """Represents system-wide context"""
    timestamp: str
    total_containers: int
    containers_at_risk: int
    critical_anomalies: int
    system_wide_trends: Dict[str, Any]
    recommendations: List[str]
    containers: Dict[str, ContainerContext]


class ContextService:
    """Service for loading and querying context"""

    def __init__(self, context_dir: str = "./context-output"):
        """
        Initialize the context service
        
        Args:
            context_dir: Directory containing context JSON files
        """
        self.context_dir = Path(context_dir)
        self.current_context: Optional[GlobalContext] = None
        self.context_history: List[GlobalContext] = []

    def load_latest_context(self) -> Optional[GlobalContext]:
        """Load the most recent context file"""
        if not self.context_dir.exists():
            print(f"Context directory not found: {self.context_dir}")
            return None

        context_files = sorted(self.context_dir.glob("context_*.json"), reverse=True)
        if not context_files:
            print("No context files found")
            return None

        latest_file = context_files[0]
        return self.load_context_file(str(latest_file))

    def load_context_file(self, filepath: str) -> Optional[GlobalContext]:
        """Load context from a specific file"""
        try:
            with open(filepath, 'r') as f:
                data = json.load(f)
            
            self.current_context = self._parse_global_context(data)
            self.context_history.append(self.current_context)
            return self.current_context
        except Exception as e:
            print(f"Error loading context file {filepath}: {e}")
            return None

    def _parse_global_context(self, data: Dict) -> GlobalContext:
        """Parse JSON data into GlobalContext"""
        containers = {}
        for container_id, container_data in data.get("containers", {}).items():
            containers[container_id] = self._parse_container_context(container_data)

        return GlobalContext(
            timestamp=data.get("timestamp", ""),
            total_containers=data.get("total_containers", 0),
            containers_at_risk=data.get("containers_at_risk", 0),
            critical_anomalies=data.get("critical_anomalies", 0),
            system_wide_trends=data.get("system_wide_trends", {}),
            recommendations=data.get("recommendations", []),
            containers=containers
        )

    def _parse_container_context(self, data: Dict) -> ContainerContext:
        """Parse container context data"""
        utilization = data.get("utilization", {})
        
        return ContainerContext(
            identity=data.get("identity", ""),
            namespace=data.get("namespace", ""),
            pod_name=data.get("pod_name", ""),
            container_name=data.get("container_name", ""),
            risk_level=data.get("risk_level", "low"),
            health_indicators=data.get("health_indicators", {}),
            anomalies=data.get("anomalies", []),
            recommendations=data.get("recommendations", []),
            utilization=utilization,
            cpu_usage=utilization.get("cpu_usage_percent", 0),
            memory_usage=utilization.get("memory_usage_percent", 0),
            disk_io_activity=utilization.get("disk_io_activity_percent", 0),
            network_busy=utilization.get("network_busy_percent", 0),
            incident_context=data.get("incident_context", {}),
        )

    # Query APIs for the Agent

    def get_containers_by_risk_level(self, level: str) -> List[ContainerContext]:
        """Get containers at a specific risk level"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.risk_level == level
        ]

    def get_critical_containers(self) -> List[ContainerContext]:
        """Get all containers at critical risk"""
        return self.get_containers_by_risk_level("critical")

    def get_high_risk_containers(self) -> List[ContainerContext]:
        """Get all containers at high risk"""
        return self.get_containers_by_risk_level("high")

    def get_containers_with_anomalies(self) -> List[ContainerContext]:
        """Get containers with detected anomalies"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.anomalies
        ]

    def get_top_anomalies_by_severity(self, limit: int = 10) -> List[Dict[str, Any]]:
        """Get top anomalies sorted by severity"""
        if not self.current_context:
            return []
        
        all_anomalies = []
        for container in self.current_context.containers.values():
            if container.anomalies:
                for anomaly in container.anomalies:
                    anomaly["container_id"] = container.identity
                    all_anomalies.append(anomaly)
        
        severity_order = {"critical": 0, "high": 1, "medium": 2, "low": 3}
        sorted_anomalies = sorted(
            all_anomalies,
            key=lambda x: severity_order.get(x.get("severity", "low"), 4)
        )
        
        return sorted_anomalies[:limit]

    def get_high_cpu_usage_containers(self, threshold: float = 60.0) -> List[ContainerContext]:
        """Get containers with CPU usage above threshold"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.cpu_usage > threshold
        ]

    def get_high_memory_usage_containers(self, threshold: float = 70.0) -> List[ContainerContext]:
        """Get containers with memory usage above threshold"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.memory_usage > threshold
        ]

    def get_increasing_trend_containers(self) -> List[ContainerContext]:
        """Get containers showing increasing resource usage trend"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.health_indicators.get("trend") == "increasing"
        ]

    def get_system_health_summary(self) -> Dict[str, Any]:
        """Get system-wide health summary"""
        if not self.current_context:
            return {}
        
        return {
            "timestamp": self.current_context.timestamp,
            "total_containers": self.current_context.total_containers,
            "containers_at_risk": self.current_context.containers_at_risk,
            "critical_anomalies": self.current_context.critical_anomalies,
            "critical_containers": len(self.get_critical_containers()),
            "high_risk_containers": len(self.get_high_risk_containers()),
            "system_wide_trends": self.current_context.system_wide_trends,
            "recommendations": self.current_context.recommendations,
        }

    def get_container_context(self, container_id: str) -> Optional[ContainerContext]:
        """Get context for a specific container"""
        if not self.current_context:
            return None
        return self.current_context.containers.get(container_id)

    def get_containers_by_namespace(self, namespace: str) -> List[ContainerContext]:
        """Get all containers in a specific namespace"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.namespace == namespace
        ]

    def get_containers_by_pod(self, pod_name: str) -> List[ContainerContext]:
        """Get all containers in a specific pod"""
        if not self.current_context:
            return []
        
        return [
            container for container in self.current_context.containers.values()
            if container.pod_name == pod_name
        ]

    def get_actionable_insights(self) -> Dict[str, List[str]]:
        """Generate actionable insights for the agent"""
        insights = {
            "immediate_actions": [],
            "monitoring_focus": [],
            "optimization_opportunities": [],
            "scaling_recommendations": [],
        }

        if not self.current_context:
            return insights

        # Immediate actions
        critical_containers = self.get_critical_containers()
        if critical_containers:
            insights["immediate_actions"].append(
                f"Address {len(critical_containers)} critical containers: "
                f"{', '.join(c.identity for c in critical_containers[:3])}"
            )

        # Monitoring focus
        anomaly_containers = self.get_containers_with_anomalies()
        if anomaly_containers:
            insights["monitoring_focus"].append(
                f"Monitor {len(anomaly_containers)} containers with anomalies"
            )

        # Scaling recommendations
        high_cpu = self.get_high_cpu_usage_containers()
        if high_cpu:
            insights["scaling_recommendations"].append(
                f"Scale {len(high_cpu)} containers with high CPU usage"
            )

        high_mem = self.get_high_memory_usage_containers()
        if high_mem:
            insights["scaling_recommendations"].append(
                f"Increase memory for {len(high_mem)} containers"
            )

        increasing_trend = self.get_increasing_trend_containers()
        if increasing_trend:
            insights["optimization_opportunities"].append(
                f"Investigate {len(increasing_trend)} containers with increasing trends"
            )

        return insights

    def export_context_for_agent(self) -> Dict[str, Any]:
        """Export full context data for agent consumption"""
        if not self.current_context:
            return {}
        
        return {
            "health_summary": self.get_system_health_summary(),
            "insights": self.get_actionable_insights(),
            "critical_issues": [
                {
                    "container": c.identity,
                    "risk_level": c.risk_level,
                    "anomalies": len(c.anomalies),
                    "recommendations": c.recommendations[:3],
                }
                for c in self.get_critical_containers()
            ],
            "high_risk_issues": [
                {
                    "container": c.identity,
                    "risk_level": c.risk_level,
                    "anomalies": len(c.anomalies),
                    "main_concern": c.health_indicators,
                }
                for c in self.get_high_risk_containers()
            ],
            "top_anomalies": self.get_top_anomalies_by_severity(5),
        }


if __name__ == "__main__":
    # Example usage
    service = ContextService()
    context = service.load_latest_context()
    
    if context:
        print("=== Context Service Summary ===")
        summary = service.get_system_health_summary()
        print(f"Timestamp: {summary['timestamp']}")
        print(f"Total Containers: {summary['total_containers']}")
        print(f"Containers at Risk: {summary['containers_at_risk']}")
        print(f"Critical Anomalies: {summary['critical_anomalies']}")
        
        print("\n=== Actionable Insights ===")
        insights = service.get_actionable_insights()
        for category, items in insights.items():
            if items:
                print(f"\n{category.upper()}:")
                for item in items:
                    print(f"  - {item}")
