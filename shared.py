from dataclasses import dataclass


@dataclass(frozen=True)
class Repository:
    # An immutable dataclass to store our Repo info in
    full_name: str  # "Offset64/EOS"
    org: str        # "Offset64"
    name: str       # "EOS"
    url: str
    version: str
    language: str   # "Rust"
