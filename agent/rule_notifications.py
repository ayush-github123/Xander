"""
Rule finding notification inbox for the agent.
"""

import json
import shutil
from pathlib import Path
from typing import Any, Dict, List


class RuleNotificationInbox:
    """Reads context-engine rule notifications from a file inbox."""

    def __init__(self, inbox_dir: str):
        self.inbox_dir = Path(inbox_dir)
        self.processed_dir = self.inbox_dir / "processed"

    def pending(self) -> List[Path]:
        if not self.inbox_dir.exists():
            return []
        return sorted(
            path for path in self.inbox_dir.glob("rule_findings_*.json")
            if path.is_file()
        )

    def load(self, path: Path) -> Dict[str, Any]:
        with open(path, "r") as f:
            return json.load(f)

    def mark_processed(self, path: Path) -> Path:
        self.processed_dir.mkdir(parents=True, exist_ok=True)
        dest = self.processed_dir / path.name
        if dest.exists():
            dest.unlink()
        shutil.move(str(path), str(dest))
        return dest
