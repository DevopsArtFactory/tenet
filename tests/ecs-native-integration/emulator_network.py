"""Create an isolated network for the Git recovery test on the loopback emulator."""
import json
import os
from pathlib import Path
import subprocess
import uuid


def create_network():
    state = Path(os.environ['TENET_EMULATOR_STATE']).resolve()
    env = {k: v for k, v in os.environ.items() if not k.startswith('AWS_')}
    env.update(AWS_SHARED_CREDENTIALS_FILE=str(state / 'emulator_credentials'),
               AWS_CONFIG_FILE=os.devnull, AWS_CA_BUNDLE=str(state / 'ca.pem'),
               AWS_EC2_METADATA_DISABLED='true', AWS_MAX_ATTEMPTS='1')

    def call(service, operation, **params):
        output = subprocess.check_output([
            'aws', '--region', 'ap-northeast-2', '--endpoint-url', 'https://127.0.0.1:19297',
            '--output', 'json', '--no-cli-pager', service, operation,
            '--cli-input-json', json.dumps(params)], env=env, timeout=20)
        return json.loads(output) if output.strip() else {}

    def ec2(operation, **params):
        return call('ec2', operation, **params)

    def elb(operation, **params):
        return call('elbv2', operation, **params)

    name = 'tenet-' + uuid.uuid4().hex[:10]
    vpc = ec2('create-vpc', CidrBlock='10.83.0.0/16')['Vpc']['VpcId']
    subnets = [ec2('create-subnet', VpcId=vpc, CidrBlock=f'10.83.{i}.0/24',
                   AvailabilityZone='ap-northeast-2' + zone)['Subnet']['SubnetId']
               for i, zone in enumerate('ab')]
    gateway = ec2('create-internet-gateway')['InternetGateway']['InternetGatewayId']
    ec2('attach-internet-gateway', VpcId=vpc, InternetGatewayId=gateway)
    table = ec2('create-route-table', VpcId=vpc)['RouteTable']['RouteTableId']
    ec2('create-route', RouteTableId=table, DestinationCidrBlock='0.0.0.0/0', GatewayId=gateway)
    for subnet in subnets:
        ec2('associate-route-table', RouteTableId=table, SubnetId=subnet)
    security = ec2('create-security-group', GroupName=name, Description='Emulated ALB targets',
                   VpcId=vpc)['GroupId']
    ec2('authorize-security-group-ingress', GroupId=security, IpPermissions=[{
        'IpProtocol': 'tcp', 'FromPort': 8080, 'ToPort': 8080,
        'IpRanges': [{'CidrIp': '10.83.0.0/16'}]}])
    lb = elb('create-load-balancer', Name=name, Subnets=subnets)['LoadBalancers'][0]
    groups = [elb('create-target-group', Name=name + color, Protocol='HTTP', Port=8080, HealthCheckIntervalSeconds=5, HealthyThresholdCount=2, HealthCheckTimeoutSeconds=2,
                  TargetType='ip', VpcId=vpc)['TargetGroups'][0]['TargetGroupArn']
              for color in ['-blue', '-green']]
    for group in groups:
        elb('modify-target-group-attributes', TargetGroupArn=group, Attributes=[
            {'Key': 'deregistration_delay.timeout_seconds', 'Value': '1'}])
    listener = elb('create-listener', LoadBalancerArn=lb['LoadBalancerArn'], Protocol='HTTP',
                   Port=80, DefaultActions=[{'Type': 'forward', 'TargetGroupArn': groups[0]}])
    targets = [{'TargetGroupArn': arn, 'Weight': weight} for arn, weight in zip(groups, [100, 0])]
    rule = elb('create-rule', ListenerArn=listener['Listeners'][0]['ListenerArn'], Priority=1,
               Conditions=[{'Field': 'path-pattern', 'Values': ['/*']}],
               Actions=[{'Type': 'forward', 'ForwardConfig': {'TargetGroups': targets}}])
    return dict(name=name, security=security, subnets=subnets, groups=groups, rule=rule['Rules'][0]['RuleArn'])


if __name__ == '__main__':
    print(json.dumps(create_network()))
