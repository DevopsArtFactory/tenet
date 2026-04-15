# Tenet v1 Spec Shapes

This document captures the first in-repo shape of Tenet's declarative resources.

## Account

```json
{
  "name": "prod-core",
  "awsAccountId": "123456789012",
  "roleArn": "arn:aws:iam::123456789012:role/TenetDeployerRole",
  "observerRoleArn": "arn:aws:iam::123456789012:role/TenetObserverRole",
  "allowedRegions": ["ap-northeast-2"]
}
```

## Environment

```json
{
  "name": "prod-apne2",
  "accountRef": "prod-core",
  "region": "ap-northeast-2",
  "networkRef": "prod-main",
  "policyRef": "prod-default"
}
```

## Deployment

```json
{
  "name": "payments-api-prod",
  "appRef": "payments-api",
  "environmentRef": "prod-apne2",
  "target": {"type": "ec2"},
  "strategy": {"type": "blueGreen", "warmupSeconds": 60, "minHealthyPercent": 100},
  "traffic": {"hostname": "api.example.com", "port": 443},
  "capacity": {"desired": 4, "min": 2, "max": 8}
}
```

These shapes are intentionally parser-agnostic so the next slice can add Git/YAML ingestion without changing validation semantics.
