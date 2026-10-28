#!/usr/bin/env python3
"""Offline repository export and explicitly invoked, read-only AWS evidence collection."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess

STRATEGIES = ('blue-green', 'canary', 'linear')


def export_repository(outputs, destination):
    files = outputs['repository_files']['value']
    expected = {'accounts/integration.yaml', 'environments/integration.yaml'} | {
        f'deployments/{key}.yaml' for key in STRATEGIES
    }
    if set(files) != expected:
        raise ValueError('Unexpected repository file set')
    documents = {name: json.loads(body) for name, body in files.items()}
    # Refuse an existing destination, including symlinks; never overwrite a Git checkout.
    destination.mkdir(parents=True, exist_ok=False)
    for name, document in documents.items():
        target = destination / name
        target.parent.mkdir(exist_ok=True)
        target.write_text(json.dumps(document, indent=2) + '\n')


def collect(bindings, strategy, destination, profile):
    service = bindings['services'][strategy]

    def read(aws_service, action, **parameters):
        command = ['aws', '--profile', profile, '--region', bindings['region'],
                   '--no-cli-pager', '--output', 'json', aws_service, action]
        for key, value in parameters.items():
            command.append('--' + key.replace('_', '-'))
            command.extend(value if isinstance(value, list) else [value])
        result = subprocess.run(command, check=True, capture_output=True, text=True, timeout=120)
        return json.loads(result.stdout)

    identity = read('sts', 'get-caller-identity')
    if identity['Account'] != bindings['account_id']:
        raise ValueError('Evidence profile must select the fixture account')
    destination.mkdir(parents=True, exist_ok=False)

    def save(name, value):
        (destination / (name + '.json')).write_text(json.dumps(value, indent=2) + '\n')
        if isinstance(value, dict) and value.get('failures'):
            raise ValueError(f'Incomplete {name} evidence')
        return value

    save('identity', identity)
    save('metadata', {'strategy': strategy, 'bindings': bindings,
                      'started_at': datetime.now(timezone.utc).isoformat()})
    state = save('service', read('ecs', 'describe-services', cluster=bindings['cluster_arn'],
                                services=[service['service_arn']]))
    if len(state.get('services', [])) != 1:
        raise ValueError('Missing service evidence')
    history = save('deployments', read('ecs', 'list-service-deployments',
                                     cluster=bindings['cluster_arn'], service=service['service_arn']))
    # AWS DescribeServiceDeployments accepts at most 20 ARNs; preserve complete history in batches.
    arns = [item['serviceDeploymentArn'] for item in history.get('serviceDeployments', [])]
    revisions = set()
    for offset in range(0, len(arns), 20):
        deployments = save(f'deployments-{offset}', read('ecs', 'describe-service-deployments',
                                                       service_deployment_arns=arns[offset:offset + 20]))
        for deployment in deployments.get('serviceDeployments', []):
            for revision in deployment.get('sourceServiceRevisions', []) + [deployment['targetServiceRevision']]:
                revisions.add(revision['arn'])
    revisions = sorted(revisions)
    for offset in range(0, len(revisions), 20):
        save(f'revisions-{offset}', read('ecs', 'describe-service-revisions',
                                         service_revision_arns=revisions[offset:offset + 20]))
    tasks = save('tasks', read('ecs', 'list-tasks', cluster=bindings['cluster_arn'],
                               service_name=service['service_arn'].rsplit('/', 1)[1]))['taskArns']
    for offset in range(0, len(tasks), 100):
        save(f'tasks-{offset}', read('ecs', 'describe-tasks', cluster=bindings['cluster_arn'],
                                     tasks=tasks[offset:offset + 100]))
    save('rules', read('elbv2', 'describe-rules', rule_arns=[service['rule_arn']]))
    for index, arn in enumerate(service['target_group_arns']):
        save(f'targets-{index}', read('elbv2', 'describe-target-health', target_group_arn=arn))
    save('alarms', read('cloudwatch', 'describe-alarms', alarm_names=[service['alarm_name']]))
    # A partial/failed collection has no completion marker and cannot be accepted as a passing run.
    save('complete', {'finished_at': datetime.now(timezone.utc).isoformat()})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['export', 'collect'])
    parser.add_argument('--outputs', required=True, type=Path, help='Saved terraform output -json')
    parser.add_argument('--destination', required=True, type=Path, help='New directory only')
    parser.add_argument('--strategy', choices=STRATEGIES)
    parser.add_argument('--profile')
    args = parser.parse_args()
    outputs = json.loads(args.outputs.read_text())
    if args.action == 'export':
        export_repository(outputs, args.destination)
    else:
        if not args.strategy or not args.profile:
            parser.error('collect requires --strategy and --profile')
        collect(outputs['test_bindings']['value'], args.strategy, args.destination, args.profile)


if __name__ == '__main__':
    main()
