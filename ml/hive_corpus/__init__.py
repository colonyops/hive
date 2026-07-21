"""Offline corpus and model pipeline for Hive terminal-status inference.

This package is developer-facing tooling. It reads the content-addressed
capture recordings that Hive writes to disk, validates their integrity,
correlates controlled-lab scenario events, manages append-only annotations,
drives LLM silver-labeling and human review, and exports training datasets.

The shipped production detector is a separate small Go runtime; nothing in
this package ships inside the Hive binary.
"""

__version__ = "0.1.0"
