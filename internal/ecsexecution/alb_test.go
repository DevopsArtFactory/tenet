package ecsexecution

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/manifest"
	"github.com/DevopsArtFactory/tenet/internal/manifestfs"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

type albFake struct {
	t                                 *testing.T
	lb                                elbtypes.LoadBalancer
	listeners                         []elbtypes.Listener
	groups                            []elbtypes.TargetGroup
	rules                             []elbtypes.Rule
	calls                             int
	unavailable                       string
	listenerARNs, ruleARNs, groupARNs []string
}

func (f *albFake) DescribeLoadBalancers(_ context.Context, input *elb.DescribeLoadBalancersInput, _ ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error) {
	f.calls++
	if f.unavailable == "lb" {
		return nil, errors.New("throttled")
	}
	if len(input.LoadBalancerArns) != 1 || input.LoadBalancerArns[0] != aws.ToString(f.lb.LoadBalancerArn) {
		f.t.Fatal("ALB preflight made an unscoped discovery call")
	}
	return &elb.DescribeLoadBalancersOutput{LoadBalancers: []elbtypes.LoadBalancer{f.lb}}, nil
}
func (f *albFake) DescribeListeners(_ context.Context, input *elb.DescribeListenersInput, _ ...func(*elb.Options)) (*elb.DescribeListenersOutput, error) {
	f.calls++
	if f.unavailable == "listeners" {
		return nil, errors.New("denied")
	}
	if !reflect.DeepEqual(input.ListenerArns, f.listenerARNs) {
		f.t.Fatal("preflight requested unreviewed listeners")
	}
	return &elb.DescribeListenersOutput{Listeners: f.listeners}, nil
}
func (f *albFake) DescribeTargetGroups(_ context.Context, input *elb.DescribeTargetGroupsInput, _ ...func(*elb.Options)) (*elb.DescribeTargetGroupsOutput, error) {
	f.calls++
	if f.unavailable == "groups" {
		return nil, errors.New("unavailable")
	}
	if !reflect.DeepEqual(input.TargetGroupArns, f.groupARNs) {
		f.t.Fatal("preflight requested unreviewed target groups")
	}
	return &elb.DescribeTargetGroupsOutput{TargetGroups: f.groups}, nil
}
func (f *albFake) DescribeRules(_ context.Context, input *elb.DescribeRulesInput, _ ...func(*elb.Options)) (*elb.DescribeRulesOutput, error) {
	f.calls++
	if f.unavailable == "rules" {
		return nil, errors.New("unavailable")
	}
	if !reflect.DeepEqual(input.RuleArns, f.ruleARNs) {
		f.t.Fatal("preflight requested unreviewed rules")
	}
	return &elb.DescribeRulesOutput{Rules: f.rules}, nil
}
func albFixture(t *testing.T) (*albFake, ALBReadiness, domain.ECSDeploymentIntent, applyrequest.DeployerRoleReadiness) {
	t.Helper()
	raw, err := manifestfs.Load(filepath.Join("..", "..", "examples", "ecs-native-advanced-repo"))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := manifest.Compile(raw)
	if err != nil {
		t.Fatal(err)
	}
	var intent domain.ECSDeploymentIntent
	for _, d := range compiled.Deployments {
		if d.Name == "web-canary" {
			intent = *d.ECS.Clone()
		}
	}
	routing := intent.Advanced.Routing
	production, _ := arn.Parse(routing.ProductionListenerRuleARN)
	test, _ := arn.Parse(routing.TestListenerRuleARN)
	lb, _ := arn.Parse("arn:aws:elasticloadbalancing:ap-northeast-2:123456789012:loadbalancer/app/tenet-example/1111111111111111")
	f := &albFake{t: t, lb: elbtypes.LoadBalancer{LoadBalancerArn: aws.String(lb.String()), Type: elbtypes.LoadBalancerTypeEnumApplication, State: &elbtypes.LoadBalancerState{Code: elbtypes.LoadBalancerStateEnumActive}, VpcId: aws.String("vpc-reviewed")}, listenerARNs: []string{albListenerForRule(production), albListenerForRule(test)}, groupARNs: []string{routing.TargetGroupARN, routing.AlternateTargetGroupARN}, ruleARNs: []string{routing.ProductionListenerRuleARN, routing.TestListenerRuleARN}}
	slices.Sort(f.listenerARNs)
	for _, value := range f.listenerARNs {
		f.listeners = append(f.listeners, elbtypes.Listener{ListenerArn: aws.String(value), LoadBalancerArn: aws.String(lb.String()), Protocol: elbtypes.ProtocolEnumHttps})
	}
	for _, value := range f.groupARNs {
		f.groups = append(f.groups, elbtypes.TargetGroup{TargetGroupArn: aws.String(value), VpcId: aws.String("vpc-reviewed"), TargetType: elbtypes.TargetTypeEnumIp, Protocol: elbtypes.ProtocolEnumHttp, Port: aws.Int32(routing.ContainerPort), LoadBalancerArns: []string{lb.String()}})
	}
	for i, value := range f.ruleARNs {
		target := routing.TargetGroupARN
		if i == 1 {
			target = routing.AlternateTargetGroupARN
		}
		f.rules = append(f.rules, elbtypes.Rule{RuleArn: aws.String(value), IsDefault: aws.Bool(false), Actions: []elbtypes.Action{{Type: elbtypes.ActionTypeEnumForward, TargetGroupArn: aws.String(target)}}})
	}
	role := applyrequest.DeployerRoleReadiness{Configured: true, Ready: true, CheckedWithAWS: true, RoleARN: "arn:aws:iam::123456789012:role/tenet-deployer", AWSAccountID: intent.AWSAccountID, Region: intent.Region}
	readiness := ALBReadiness{ClientFor: func(context.Context, applyrequest.DeployerRoleReadiness) (ALBClient, error) { return f, nil }, CheckCapabilities: func(_ context.Context, role applyrequest.DeployerRoleReadiness, checks []applyrequest.OperationExecutionCapabilityCheck) error {
		if len(checks) != 4 {
			t.Fatal("missing concrete read checks")
		}
		for _, check := range checks {
			if check.ResourceName != "*" || check.RequiredRole != role.RoleARN || !slices.Contains([]string{"elasticloadbalancing:DescribeLoadBalancers", "elasticloadbalancing:DescribeListeners", "elasticloadbalancing:DescribeRules", "elasticloadbalancing:DescribeTargetGroups"}, check.Action) {
				t.Fatal("routing probe requested mutation capabilities")
			}
		}
		return nil
	}}
	return f, readiness, intent, role
}
func TestALBReadinessUsesReviewedReadsAndStableRoutingFence(t *testing.T) {
	f, p, intent, role := albFixture(t)
	snapshot, err := p.Observe(context.Background(), intent, role)
	if err != nil || snapshot.VPCID != "vpc-reviewed" || snapshot.Production.Primary != 1 || snapshot.Production.Alternate != 0 || snapshot.Test == nil || snapshot.Test.Alternate != 1 || snapshot.Signature == "" || f.calls != 4 {
		t.Fatalf("missing routing evidence: %+v %v", snapshot, err)
	}
	again, err := p.Observe(context.Background(), intent, role)
	if err != nil || again.Signature != snapshot.Signature {
		t.Fatal("unchanged routing fence churned")
	}
	// Production can be weighted during AWS-managed progression. These are config
	// weights only; they do not claim that measured request traffic has shifted.
	f.rules[0].Actions[0] = elbtypes.Action{Type: elbtypes.ActionTypeEnumForward, ForwardConfig: &elbtypes.ForwardActionConfig{TargetGroups: []elbtypes.TargetGroupTuple{{TargetGroupArn: aws.String(intent.Advanced.Routing.TargetGroupARN), Weight: aws.Int32(95)}, {TargetGroupArn: aws.String(intent.Advanced.Routing.AlternateTargetGroupARN), Weight: aws.Int32(5)}}}}
	weighted, err := p.Observe(context.Background(), intent, role)
	if err != nil || weighted.Production.Primary != 95 || weighted.Production.Alternate != 5 || weighted.Signature == snapshot.Signature {
		t.Fatal("weighted routing lost from final mutation fence")
	}
	slices.Reverse(f.rules)
	slices.Reverse(f.groups)
	slices.Reverse(f.listeners)
	reordered, err := p.Observe(context.Background(), intent, role)
	if err != nil || reordered.Signature != weighted.Signature {
		t.Fatal("AWS response ordering invalidated routing fence")
	}
}
func TestALBReadinessRejectsUnknownForeignAndUnsupportedRouting(t *testing.T) {
	for _, name := range []string{"unverified role", "cross account role", "lb unavailable", "listeners denied", "groups unavailable", "rules unavailable", "NLB", "inactive LB", "missing listener", "wrong listener LB", "missing target", "duplicate target", "wrong VPC", "instance targets", "wrong port", "TCP target", "foreign LB attachment", "missing attachment", "missing rule", "default rule", "foreign forward target", "zero weights", "missing observed weight", "sticky pair", "redirect", "multiple forwards", "permission denied", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			f, p, intent, role := albFixture(t)
			ctx := context.Background()
			switch name {
			case "unverified role":
				role.CheckedWithAWS = false
			case "cross account role":
				role.RoleARN = "arn:aws:iam::999999999999:role/other"
			case "lb unavailable":
				f.unavailable = "lb"
			case "listeners denied":
				f.unavailable = "listeners"
			case "groups unavailable":
				f.unavailable = "groups"
			case "rules unavailable":
				f.unavailable = "rules"
			case "NLB":
				f.lb.Type = elbtypes.LoadBalancerTypeEnumNetwork
			case "inactive LB":
				f.lb.State.Code = elbtypes.LoadBalancerStateEnumProvisioning
			case "missing listener":
				f.listeners = nil
			case "wrong listener LB":
				f.listeners[0].LoadBalancerArn = aws.String("foreign")
			case "missing target":
				f.groups = f.groups[:1]
			case "duplicate target":
				f.groups[1] = f.groups[0]
			case "wrong VPC":
				f.groups[1].VpcId = aws.String("other")
			case "instance targets":
				f.groups[1].TargetType = elbtypes.TargetTypeEnumInstance
			case "wrong port":
				f.groups[1].Port = aws.Int32(9999)
			case "TCP target":
				f.groups[1].Protocol = elbtypes.ProtocolEnumTcp
			case "foreign LB attachment":
				f.groups[1].LoadBalancerArns = []string{"foreign"}
			case "missing attachment":
				f.groups[0].LoadBalancerArns = nil
			case "missing rule":
				f.rules = nil
			case "default rule":
				f.rules[0].IsDefault = aws.Bool(true)
			case "foreign forward target":
				f.rules[0].Actions[0].TargetGroupArn = aws.String("foreign")
			case "zero weights":
				f.rules[0].Actions[0] = elbtypes.Action{Type: elbtypes.ActionTypeEnumForward, ForwardConfig: &elbtypes.ForwardActionConfig{TargetGroups: []elbtypes.TargetGroupTuple{{TargetGroupArn: aws.String(intent.Advanced.Routing.TargetGroupARN), Weight: aws.Int32(0)}}}}
			case "missing observed weight":
				f.rules[0].Actions[0] = elbtypes.Action{Type: elbtypes.ActionTypeEnumForward, ForwardConfig: &elbtypes.ForwardActionConfig{TargetGroups: []elbtypes.TargetGroupTuple{{TargetGroupArn: aws.String(intent.Advanced.Routing.TargetGroupARN)}}}}
			case "sticky pair":
				f.rules[0].Actions[0] = elbtypes.Action{Type: elbtypes.ActionTypeEnumForward, ForwardConfig: &elbtypes.ForwardActionConfig{TargetGroups: []elbtypes.TargetGroupTuple{{TargetGroupArn: aws.String(intent.Advanced.Routing.TargetGroupARN), Weight: aws.Int32(1)}}, TargetGroupStickinessConfig: &elbtypes.TargetGroupStickinessConfig{Enabled: aws.Bool(true)}}}
			case "redirect":
				f.rules[0].Actions[0].Type = elbtypes.ActionTypeEnumRedirect
			case "multiple forwards":
				f.rules[0].Actions = append(f.rules[0].Actions, f.rules[0].Actions[0])
			case "permission denied":
				p.CheckCapabilities = func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error {
					return errors.New("denied")
				}
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			snapshot, err := p.Observe(ctx, intent, role)
			if err == nil || snapshot.Signature != "" {
				t.Fatalf("unsafe routing marked ready: %+v %v", snapshot, err)
			}
			if (name == "permission denied" || name == "cancelled" || name == "unverified role" || name == "cross account role") && f.calls != 0 {
				t.Fatal("untrusted routing probe contacted ALB")
			}
		})
	}
}

func TestALBReadinessHandlesOptionalTestRuleAndFencesConfigChanges(t *testing.T) {
	f, p, intent, role := albFixture(t)
	intent.Advanced.Routing.TestListenerRuleARN = ""
	f.listenerARNs = f.listenerARNs[:1]
	f.listeners = f.listeners[:1]
	f.ruleARNs = f.ruleARNs[:1]
	f.rules = f.rules[:1]
	snapshot, err := p.Observe(context.Background(), intent, role)
	if err != nil || snapshot.Test != nil {
		t.Fatal("optional test route invented evidence")
	}
	f.groups[0].HealthCheckPath = aws.String("/changed-health")
	changed, err := p.Observe(context.Background(), intent, role)
	if err != nil || changed.Signature == snapshot.Signature {
		t.Fatal("changed target health configuration escaped pre-mutation fence")
	}
}
