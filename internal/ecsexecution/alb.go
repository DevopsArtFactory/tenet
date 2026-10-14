package ecsexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/DevopsArtFactory/tenet/internal/applyrequest"
	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/DevopsArtFactory/tenet/internal/spec"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	elb "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
)

// ALBClient deliberately exposes read calls only. ECS owns traffic mutations.
type ALBClient interface {
	DescribeLoadBalancers(context.Context, *elb.DescribeLoadBalancersInput, ...func(*elb.Options)) (*elb.DescribeLoadBalancersOutput, error)
	DescribeListeners(context.Context, *elb.DescribeListenersInput, ...func(*elb.Options)) (*elb.DescribeListenersOutput, error)
	DescribeRules(context.Context, *elb.DescribeRulesInput, ...func(*elb.Options)) (*elb.DescribeRulesOutput, error)
	DescribeTargetGroups(context.Context, *elb.DescribeTargetGroupsInput, ...func(*elb.Options)) (*elb.DescribeTargetGroupsOutput, error)
}
type ALBReadiness struct {
	ClientFor         func(context.Context, applyrequest.DeployerRoleReadiness) (ALBClient, error)
	CheckCapabilities func(context.Context, applyrequest.DeployerRoleReadiness, []applyrequest.OperationExecutionCapabilityCheck) error
}

// Weights are ALB routing weights, not measurements of actual HTTP requests.
type ALBWeightPair struct {
	Primary   int32 `json:"primary"`
	Alternate int32 `json:"alternate"`
}
type ALBRoutingSnapshot struct {
	LoadBalancerARN string         `json:"loadBalancerArn"`
	VPCID           string         `json:"vpcId"`
	Production      ALBWeightPair  `json:"production"`
	Test            *ALBWeightPair `json:"test,omitempty"`
	Signature       string         `json:"signature"`
}

func (p ALBReadiness) Observe(ctx context.Context, intent domain.ECSDeploymentIntent, role applyrequest.DeployerRoleReadiness) (ALBRoutingSnapshot, error) {
	var empty ALBRoutingSnapshot
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if intent.Advanced == nil || p.ClientFor == nil || p.CheckCapabilities == nil {
		return empty, fmt.Errorf("reviewed advanced routing and deployer ALB providers are required")
	}
	// Reuse manifest placement/contract validation for callers of this standalone probe.
	strategy := map[string]spec.StrategyType{"BLUE_GREEN": spec.StrategyTypeBlueGreen, "CANARY": spec.StrategyTypeCanary, "LINEAR": spec.StrategyTypeLinear}[intent.DeploymentStrategy]
	if strategy == "" || intent.DeploymentController != "ECS" {
		return empty, fmt.Errorf("ALB readiness requires reviewed native advanced intent")
	}
	contract := spec.DeploymentSpec{Name: "routing-check", EnvironmentRef: "reviewed", Target: spec.TargetSpec{Type: spec.TargetTypeECS}, Strategy: spec.StrategySpec{Type: strategy}, Image: intent.Image, Capacity: spec.CapacitySpec{Desired: intent.DesiredCount, Min: intent.DesiredCount, Max: intent.DesiredCount}, ECS: &spec.ECSDeploymentSpec{DeploymentController: "ECS", ClusterARN: intent.ClusterARN, ServiceARN: intent.ServiceARN, BaseTaskDefinitionARN: intent.BaseTaskDefinitionARN, ContainerName: intent.ContainerName, Advanced: (*spec.ECSAdvancedSpec)(intent.Advanced)}}
	if err := spec.ValidateDeployment(contract); err != nil {
		return empty, err
	}
	placement, err := arn.Parse(intent.ServiceARN)
	principal, roleErr := arn.Parse(role.RoleARN)
	if err != nil || roleErr != nil || placement.AccountID != intent.AWSAccountID || placement.Region != intent.Region || principal.Service != "iam" || !strings.HasPrefix(principal.Resource, "role/") || principal.AccountID != intent.AWSAccountID || principal.Partition != placement.Partition || !role.Configured || !role.Ready || !role.CheckedWithAWS || role.AWSAccountID != intent.AWSAccountID || role.Region != intent.Region {
		return empty, fmt.Errorf("ALB readiness requires verified service placement and deployer role")
	}
	routing := intent.Advanced.Routing
	production, _ := arn.Parse(routing.ProductionListenerRuleARN)
	parts := strings.Split(production.Resource, "/")
	lb := production
	lb.Resource = "loadbalancer/" + strings.Join(parts[1:4], "/")
	listeners := []string{albListenerForRule(production)}
	ruleARNs := []string{routing.ProductionListenerRuleARN}
	if routing.TestListenerRuleARN != "" {
		test, _ := arn.Parse(routing.TestListenerRuleARN)
		listeners = append(listeners, albListenerForRule(test))
		ruleARNs = append(ruleARNs, routing.TestListenerRuleARN)
	}
	slices.Sort(listeners)
	listeners = slices.Compact(listeners)
	actions := []string{"DescribeLoadBalancers", "DescribeListeners", "DescribeRules", "DescribeTargetGroups"}
	checks := make([]applyrequest.OperationExecutionCapabilityCheck, 0, len(actions))
	for _, action := range actions {
		checks = append(checks, applyrequest.OperationExecutionCapabilityCheck{Action: "elasticloadbalancing:" + action, ResourceName: "*", RequiredRole: role.RoleARN})
	}
	// ELB describe actions have no resource-level IAM support. Actual requests below
	// still select only the explicitly reviewed ARNs, never account-wide discovery.
	if err := p.CheckCapabilities(ctx, role, checks); err != nil {
		return empty, err
	}
	client, err := p.ClientFor(ctx, role)
	if err != nil {
		return empty, err
	}
	if client == nil {
		return empty, fmt.Errorf("deployer ALB client unavailable")
	}
	lbs, err := client.DescribeLoadBalancers(ctx, &elb.DescribeLoadBalancersInput{LoadBalancerArns: []string{lb.String()}})
	if err != nil {
		return empty, err
	}
	if lbs == nil || len(lbs.LoadBalancers) != 1 {
		return empty, fmt.Errorf("reviewed ALB unavailable")
	}
	observedLB := lbs.LoadBalancers[0]
	if aws.ToString(observedLB.LoadBalancerArn) != lb.String() || observedLB.Type != elbtypes.LoadBalancerTypeEnumApplication || observedLB.State == nil || observedLB.State.Code != elbtypes.LoadBalancerStateEnumActive || aws.ToString(observedLB.VpcId) == "" {
		return empty, fmt.Errorf("routing requires the exact active Application Load Balancer and VPC")
	}
	listenerOutput, err := client.DescribeListeners(ctx, &elb.DescribeListenersInput{ListenerArns: listeners})
	if err != nil {
		return empty, err
	}
	if listenerOutput == nil || len(listenerOutput.Listeners) != len(listeners) {
		return empty, fmt.Errorf("reviewed ALB listeners unavailable")
	}
	seen := map[string]bool{}
	metadata := map[string]any{}
	for _, listener := range listenerOutput.Listeners {
		value := aws.ToString(listener.ListenerArn)
		if !slices.Contains(listeners, value) || seen[value] || aws.ToString(listener.LoadBalancerArn) != lb.String() || (listener.Protocol != elbtypes.ProtocolEnumHttp && listener.Protocol != elbtypes.ProtocolEnumHttps) {
			return empty, fmt.Errorf("listener must match the reviewed ALB with HTTP/HTTPS protocol")
		}
		seen[value] = true
		metadata["listener:"+value] = struct {
			Protocol  elbtypes.ProtocolEnum
			Port      *int32
			SSLPolicy *string
		}{listener.Protocol, listener.Port, listener.SslPolicy}
	}
	groupOutput, err := client.DescribeTargetGroups(ctx, &elb.DescribeTargetGroupsInput{TargetGroupArns: []string{routing.TargetGroupARN, routing.AlternateTargetGroupARN}})
	if err != nil {
		return empty, err
	}
	if groupOutput == nil || len(groupOutput.TargetGroups) != 2 {
		return empty, fmt.Errorf("both reviewed target groups must exist")
	}
	groups := map[string]elbtypes.TargetGroup{}
	for _, group := range groupOutput.TargetGroups {
		value := aws.ToString(group.TargetGroupArn)
		if (value != routing.TargetGroupARN && value != routing.AlternateTargetGroupARN) || groups[value].TargetGroupArn != nil || aws.ToString(group.VpcId) != aws.ToString(observedLB.VpcId) || group.TargetType != elbtypes.TargetTypeEnumIp || aws.ToInt32(group.Port) != routing.ContainerPort || (group.Protocol != elbtypes.ProtocolEnumHttp && group.Protocol != elbtypes.ProtocolEnumHttps) {
			return empty, fmt.Errorf("target groups require reviewed IP targets, HTTP/HTTPS port and ALB VPC")
		}
		for _, associated := range group.LoadBalancerArns {
			if associated != lb.String() {
				return empty, fmt.Errorf("target group is attached to an unreviewed load balancer")
			}
		}
		metadata["target:"+value] = struct {
			Protocol                                                                  elbtypes.ProtocolEnum
			ProtocolVersion                                                           *string
			Port, HealthInterval, HealthTimeout, HealthyThreshold, UnhealthyThreshold *int32
			HealthProtocol                                                            elbtypes.ProtocolEnum
			HealthPort, HealthPath                                                    *string
			HealthEnabled                                                             *bool
			Matcher                                                                   *elbtypes.Matcher
		}{group.Protocol, group.ProtocolVersion, group.Port, group.HealthCheckIntervalSeconds, group.HealthCheckTimeoutSeconds, group.HealthyThresholdCount, group.UnhealthyThresholdCount, group.HealthCheckProtocol, group.HealthCheckPort, group.HealthCheckPath, group.HealthCheckEnabled, group.Matcher}
		groups[value] = group
	}
	if groups[routing.TargetGroupARN].Protocol != groups[routing.AlternateTargetGroupARN].Protocol || aws.ToString(groups[routing.TargetGroupARN].ProtocolVersion) != aws.ToString(groups[routing.AlternateTargetGroupARN].ProtocolVersion) {
		return empty, fmt.Errorf("target group protocols must match")
	}
	ruleOutput, err := client.DescribeRules(ctx, &elb.DescribeRulesInput{RuleArns: ruleARNs})
	if err != nil {
		return empty, err
	}
	if ruleOutput == nil || len(ruleOutput.Rules) != len(ruleARNs) {
		return empty, fmt.Errorf("reviewed ALB rules unavailable")
	}
	snapshot := ALBRoutingSnapshot{LoadBalancerARN: lb.String(), VPCID: aws.ToString(observedLB.VpcId)}
	seen = map[string]bool{}
	// Conditions/transforms are hashed for the final pre-mutation fence, never
	// persisted as task bodies, credentials or authentication configuration.
	conditions := map[string]any{}
	for _, rule := range ruleOutput.Rules {
		value := aws.ToString(rule.RuleArn)
		if !slices.Contains(ruleARNs, value) || seen[value] || aws.ToBool(rule.IsDefault) {
			return empty, fmt.Errorf("exact non-default ALB listener rules are required")
		}
		seen[value] = true
		pair, err := reviewedALBWeights(rule.Actions, routing)
		if err != nil {
			return empty, err
		}
		if pair.Primary > 0 && len(groups[routing.TargetGroupARN].LoadBalancerArns) == 0 || pair.Alternate > 0 && len(groups[routing.AlternateTargetGroupARN].LoadBalancerArns) == 0 {
			return empty, fmt.Errorf("forwarded target group attachment is unavailable")
		}
		if value == routing.ProductionListenerRuleARN {
			snapshot.Production = pair
		} else {
			copy := pair
			snapshot.Test = &copy
		}
		conditions[value] = struct {
			Conditions []elbtypes.RuleCondition
			Transforms []elbtypes.RuleTransform
		}{rule.Conditions, rule.Transforms}
	}
	content, err := json.Marshal(struct {
		Snapshot   ALBRoutingSnapshot
		Conditions map[string]any
		Metadata   map[string]any
	}{snapshot, conditions, metadata})
	if err != nil {
		return empty, err
	}
	sum := sha256.Sum256(content)
	snapshot.Signature = hex.EncodeToString(sum[:])
	return snapshot, nil
}
func albListenerForRule(rule arn.ARN) string {
	parts := strings.Split(rule.Resource, "/")
	rule.Resource = "listener/" + strings.Join(parts[1:5], "/")
	return rule.String()
}
func reviewedALBWeights(actions []elbtypes.Action, routing domain.ECSALBRouting) (ALBWeightPair, error) {
	var pair ALBWeightPair
	found := false
	for _, action := range actions {
		if action.Type == elbtypes.ActionTypeEnumAuthenticateOidc || action.Type == elbtypes.ActionTypeEnumAuthenticateCognito {
			continue
		}
		if action.Type != elbtypes.ActionTypeEnumForward || found {
			return pair, fmt.Errorf("ALB rule must have one forward action and optional authentication only")
		}
		found = true
		groups := []elbtypes.TargetGroupTuple{{TargetGroupArn: action.TargetGroupArn, Weight: aws.Int32(1)}}
		if action.ForwardConfig != nil {
			if action.ForwardConfig.TargetGroupStickinessConfig != nil && aws.ToBool(action.ForwardConfig.TargetGroupStickinessConfig.Enabled) {
				return pair, fmt.Errorf("weighted target-group stickiness is outside initial native routing support")
			}
			groups = action.ForwardConfig.TargetGroups
			for _, group := range groups {
				if group.Weight == nil {
					return pair, fmt.Errorf("observed ALB target weights are required")
				}
			}
			if action.TargetGroupArn != nil && (len(groups) != 1 || aws.ToString(groups[0].TargetGroupArn) != aws.ToString(action.TargetGroupArn)) {
				return pair, fmt.Errorf("ALB forward aliases disagree")
			}
		}
		if len(groups) == 0 || len(groups) > 2 {
			return pair, fmt.Errorf("ALB rule must forward only to the reviewed target pair")
		}
		seen := map[string]bool{}
		for _, group := range groups {
			value := aws.ToString(group.TargetGroupArn)
			weight := int32(1)
			if group.Weight != nil {
				weight = *group.Weight
			}
			if seen[value] || weight < 0 || weight > 999 || (value != routing.TargetGroupARN && value != routing.AlternateTargetGroupARN) {
				return pair, fmt.Errorf("ALB forward target/weight is unavailable or unreviewed")
			}
			seen[value] = true
			if value == routing.TargetGroupARN {
				pair.Primary = weight
			} else {
				pair.Alternate = weight
			}
		}
	}
	if !found || pair.Primary+pair.Alternate == 0 {
		return pair, fmt.Errorf("ALB forward action has no positive reviewed target weight")
	}
	return pair, nil
}
