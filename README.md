# 🔑 GitHub Pubkey Collector

Utility for collecting public SSH keys from GitHub users via public events and organizational member lists.

Currently dumps them to independent JSON files, but we'll probably add BadgerDB support.

## Prerequisites
- Go 1.23+
- GitHub Personal Access Token

## Installation

```bash
go install github.com/tstromberg/pubkey-collector@latest
```

## Usage

```bash
export GITHUB_TOKEN=your_github_token
pubkey-collector -stream        # Collect from events (infinitely)
pubkey-collector -org myorg     # Collect from organization
```
