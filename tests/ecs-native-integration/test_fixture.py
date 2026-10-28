import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import fixture


class FixtureTest(unittest.TestCase):
    def outputs(self):
        names = ['accounts/integration.yaml', 'environments/integration.yaml'] + [
            f'deployments/{key}.yaml' for key in fixture.STRATEGIES]
        return {'repository_files': {'value': {name: '{"name":"fixture"}' for name in names}}}

    def test_export_is_offline_and_never_overwrites(self):
        with tempfile.TemporaryDirectory() as root, patch('subprocess.run', side_effect=AssertionError('AWS called')):
            destination = Path(root) / 'repo'
            fixture.export_repository(self.outputs(), destination)
            self.assertEqual(len(list(destination.rglob('*.yaml'))), 5)
            with self.assertRaises(FileExistsError):
                fixture.export_repository(self.outputs(), destination)
            outputs = self.outputs()
            outputs['repository_files']['value']['../escape.yaml'] = '{}'
            with self.assertRaises(ValueError):
                fixture.export_repository(outputs, Path(root) / 'bad')
            self.assertFalse((Path(root) / 'bad').exists())

    def test_collect_only_reads_and_marks_complete(self):
        bindings = {'account_id': '123456789012', 'region': 'ap-northeast-2', 'cluster_arn': 'cluster',
                    'services': {'canary': {'service_arn': 'service/test', 'rule_arn': 'rule',
                                            'target_group_arns': ['blue', 'green'], 'alarm_name': 'alarm'}}}
        responses = {'get-caller-identity': {'Account': '123456789012'},
                     'describe-services': {'services': [{}]},
                     'list-service-deployments': {'serviceDeployments': [{'serviceDeploymentArn': 'deployment'}]},
                     'describe-service-deployments': {'serviceDeployments': [{
                         'sourceServiceRevisions': [{'arn': 'source'}], 'targetServiceRevision': {'arn': 'target'}}]},
                     'list-tasks': {'taskArns': ['task']}}
        allowed = {'get-caller-identity', 'describe-services', 'list-service-deployments',
                   'describe-service-deployments', 'describe-service-revisions', 'list-tasks',
                   'describe-tasks', 'describe-rules', 'describe-target-health', 'describe-alarms'}
        seen = set()

        def run(command, **kwargs):
            action = command[9]
            self.assertIn(action, allowed)
            seen.add(action)
            return type('Result', (), {'stdout': json.dumps(responses.get(action, {}))})()

        with tempfile.TemporaryDirectory() as root, patch('subprocess.run', side_effect=run):
            destination = Path(root) / 'evidence'
            fixture.collect(bindings, 'canary', destination, 'sandbox')
            self.assertTrue((destination / 'complete.json').exists())
            self.assertEqual(seen, allowed)
            responses['describe-tasks'] = {'failures': [{'reason': 'MISSING'}]}
            with self.assertRaises(ValueError):
                fixture.collect(bindings, 'canary', Path(root) / 'partial', 'sandbox')
            self.assertFalse((Path(root) / 'partial/complete.json').exists())
            bindings['account_id'] = '999999999999'
            with self.assertRaises(ValueError):
                fixture.collect(bindings, 'canary', Path(root) / 'wrong', 'sandbox')
            self.assertFalse((Path(root) / 'wrong').exists())

    def test_test_app_health_failure_is_separate_from_marker(self):
        spec = importlib.util.spec_from_file_location('test_app', Path(__file__).parent / 'app/server.py')
        app = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(app)
        from io import BytesIO
        for path, env, expected in [('/canary/', {}, 200), ('/health', {}, 200),
                                    ('/health', {'FAIL_HEALTH': 'true'}, 503)]:
            handler = object.__new__(app.Handler)
            handler.path, handler.wfile = path, BytesIO()
            with patch.dict(app.os.environ, env, clear=True), patch.object(handler, 'send_response') as response, patch.object(handler, 'end_headers'):
                handler.do_GET()
                response.assert_called_once_with(expected)
                self.assertEqual(handler.wfile.getvalue(), b'baseline\n')


if __name__ == '__main__':
    unittest.main()
