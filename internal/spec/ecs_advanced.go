package spec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/DevopsArtFactory/tenet/internal/domain"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"gopkg.in/yaml.v3"
)

// Strict decoding is scoped to the new advanced contract. Unsupported fields
// cannot silently disappear from a reviewed traffic policy.
type ECSAdvancedSpec domain.ECSAdvancedConfiguration

func (s *ECSAdvancedSpec) UnmarshalJSON(content []byte) error {
	type plain ECSAdvancedSpec
	var value plain
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode ecs.advanced: %w", err)
	}
	*s = ECSAdvancedSpec(value)
	return nil
}
func (s *ECSAdvancedSpec) UnmarshalYAML(node *yaml.Node) error {
	var value any
	if err := node.Decode(&value); err != nil {
		return err
	}
	content, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode ecs.advanced YAML: %w", err)
	}
	return s.UnmarshalJSON(content)
}
func (s *ECSAdvancedSpec) Intent() *domain.ECSAdvancedConfiguration {
	if s == nil {
		return nil
	}
	return (*domain.ECSAdvancedConfiguration)(s).Clone()
}

var ecsALBTargetGroupPattern = regexp.MustCompile(`^targetgroup/[a-zA-Z0-9-]{1,32}/[a-fA-F0-9]{16}$`)
var ecsALBRulePattern = regexp.MustCompile(`^listener-rule/app/[a-zA-Z0-9-]{1,32}/[a-fA-F0-9]{16}/[a-fA-F0-9]{16}/[a-fA-F0-9]{16}$`)
var ecsIAMRolePattern = regexp.MustCompile(`^role/(?:[a-zA-Z0-9+=,.@_-]+/)*[a-zA-Z0-9+=,.@_-]{1,64}$`)
var ecsVersionedLambdaPattern = regexp.MustCompile(`^function:[a-zA-Z0-9_-]{1,64}:[1-9][0-9]*$`)

func validateECSAdvanced(d DeploymentSpec) error {
	c := d.ECS.Advanced
	if c == nil {
		return nil
	} // Existing intent-only previews remain readable; execution stays gated.
	strategy := d.NormalizedStrategy()
	if strategy == StrategyTypeRolling {
		return fmt.Errorf("ecs.advanced requires native Blue-Green, Canary or Linear")
	}
	if d.ECS.MinimumHealthyPercent != nil || d.ECS.MaximumPercent != nil || d.Strategy.MinHealthyPercent != 0 {
		return fmt.Errorf("ecs.advanced cannot use Rolling percentage settings")
	}
	if !validECSBake(c.BakeTimeInMinutes) {
		return fmt.Errorf("ecs.advanced.bakeTimeInMinutes must explicitly specify 0..1440")
	}
	totalBake := int64(*c.BakeTimeInMinutes)
	switch strategy {
	case StrategyTypeBlueGreen:
		if c.Canary != nil || c.Linear != nil {
			return fmt.Errorf("Blue-Green cannot contain Canary or Linear settings")
		}
	case StrategyTypeCanary:
		if c.Canary == nil || c.Linear != nil || !validECSTrafficPercent(c.Canary.Percent, 0.1) || !validECSBake(c.Canary.BakeTimeInMinutes) {
			return fmt.Errorf("Canary requires percent 0.1..100 in 0.1 increments and bake time 0..1440, without Linear settings")
		}
		totalBake += int64(*c.Canary.BakeTimeInMinutes)
	case StrategyTypeLinear:
		if c.Linear == nil || c.Canary != nil || !validECSTrafficPercent(c.Linear.StepPercent, 3) || !validECSBake(c.Linear.StepBakeTimeInMinutes) {
			return fmt.Errorf("Linear requires stepPercent 3..100 in 0.1 increments and step bake time 0..1440, without Canary settings")
		}
		totalBake += int64(math.Ceil(100 / *c.Linear.StepPercent)-1) * int64(*c.Linear.StepBakeTimeInMinutes)
	default:
		return fmt.Errorf("unsupported native advanced strategy")
	}
	if totalBake > 1440 {
		return fmt.Errorf("Tenet advanced traffic/bake schedule must fit within 24 hours")
	}
	placement, err := arn.Parse(d.ECS.ClusterARN)
	if err != nil {
		return fmt.Errorf("advanced routing requires valid ECS placement")
	}
	if err := validateECSALBRouting(c.Routing, placement); err != nil {
		return err
	}
	if c.Alarms != nil {
		if !c.Alarms.Enable || len(c.Alarms.Names) == 0 || len(c.Alarms.Names) > 10 {
			return fmt.Errorf("advanced alarms must enable 1..10 named CloudWatch alarms; omit the block for none")
		}
		names := map[string]bool{}
		for _, name := range c.Alarms.Names {
			if name == "" || len(name) > 255 || strings.TrimSpace(name) != name || strings.IndexFunc(name, unicode.IsControl) >= 0 || names[name] {
				return fmt.Errorf("advanced alarm names must be nonempty, unique and at most 255 bytes")
			}
			names[name] = true
		}
	}
	stages := map[string]bool{}
	allowedStages := map[string]bool{"RECONCILE_SERVICE": true, "PRE_SCALE_UP": true, "POST_SCALE_UP": true, "TEST_TRAFFIC_SHIFT": true, "POST_TEST_TRAFFIC_SHIFT": true, "PRODUCTION_TRAFFIC_SHIFT": true, "POST_PRODUCTION_TRAFFIC_SHIFT": true}
	for _, hook := range c.LifecycleHooks {
		if !ecsPlacedARN(hook.FunctionARN, "lambda", ecsVersionedLambdaPattern, placement, false) || !ecsPlacedARN(hook.RoleARN, "iam", ecsIAMRolePattern, placement, true) || len(hook.Stages) == 0 {
			return fmt.Errorf("advanced hooks require a same-placement version-pinned Lambda, IAM role and lifecycle stages")
		}
		for _, stage := range hook.Stages {
			if !allowedStages[stage] || stages[stage] {
				return fmt.Errorf("advanced hooks support one Lambda per declared lifecycle stage; Pause/Continue and unknown stages are unsupported")
			}
			stages[stage] = true
			if (stage == "TEST_TRAFFIC_SHIFT" || stage == "POST_TEST_TRAFFIC_SHIFT") && c.Routing.TestListenerRuleARN == "" {
				return fmt.Errorf("test traffic hooks require a reviewed test listener rule")
			}
		}
	}
	return nil
}
func validECSBake(value *int32) bool { return value != nil && *value >= 0 && *value <= 1440 }
func validECSTrafficPercent(value *float64, minimum float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= minimum && *value <= 100 && math.Abs(*value*10-math.Round(*value*10)) < 1e-8
}
func ecsPlacedARN(value, service string, resource *regexp.Regexp, placement arn.ARN, global bool) bool {
	ref, err := arn.Parse(value)
	return err == nil && ref.Partition == placement.Partition && ref.AccountID == placement.AccountID && ref.Service == service && resource.MatchString(ref.Resource) && ((global && ref.Region == "") || (!global && ref.Region == placement.Region))
}
func validateECSALBRouting(r domain.ECSALBRouting, placement arn.ARN) error {
	if r.ContainerPort < 1 || r.ContainerPort > 65535 || !ecsPlacedARN(r.TargetGroupARN, "elasticloadbalancing", ecsALBTargetGroupPattern, placement, false) || !ecsPlacedARN(r.AlternateTargetGroupARN, "elasticloadbalancing", ecsALBTargetGroupPattern, placement, false) || r.TargetGroupARN == r.AlternateTargetGroupARN || !ecsPlacedARN(r.ProductionListenerRuleARN, "elasticloadbalancing", ecsALBRulePattern, placement, false) || !ecsPlacedARN(r.InfrastructureRoleARN, "iam", ecsIAMRolePattern, placement, true) {
		return fmt.Errorf("advanced routing requires a container port, two distinct same-placement ALB target groups, a production rule and infrastructure role")
	}
	if r.TestListenerRuleARN != "" {
		if !ecsPlacedARN(r.TestListenerRuleARN, "elasticloadbalancing", ecsALBRulePattern, placement, false) || r.TestListenerRuleARN == r.ProductionListenerRuleARN {
			return fmt.Errorf("test listener must be a distinct same-placement ALB rule")
		}
		production, _ := arn.Parse(r.ProductionListenerRuleARN)
		test, _ := arn.Parse(r.TestListenerRuleARN)
		if strings.Join(strings.Split(production.Resource, "/")[:4], "/") != strings.Join(strings.Split(test.Resource, "/")[:4], "/") {
			return fmt.Errorf("production/test rules must belong to the same ALB")
		}
	}
	return nil
}
