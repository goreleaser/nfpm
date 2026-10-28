package rpm

import (
	"fmt"
	"os"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"go.digitalxero.dev/rpm"
)

type trigger struct {
	triggerType rpm.TriggerKind
	directive   string
	script      string
	interpreter string
	conditions  []rpm.Relation
}

type triggerCondition struct {
	triggerType rpm.TriggerKind
	relation    rpm.Relation
	condition   string
	trigger     int
}

func readTriggers(info *nfpm.Info) ([]trigger, error) {
	triggers := make([]trigger, 0, len(info.RPM.Triggers))
	// RPM runs a single trigger script per type for a given package version,
	// and rpmbuild refuses a spec where two such conditions overlap.
	var defined []triggerCondition
	for i, t := range info.RPM.Triggers {
		triggerType, err := parseTriggerType(t.Type)
		if err != nil {
			return nil, fmt.Errorf("rpm trigger %d: %w", i+1, err)
		}

		if t.Script == "" {
			return nil, fmt.Errorf("rpm trigger %d: script must be provided", i+1)
		}

		body, err := os.ReadFile(t.Script)
		if err != nil {
			return nil, fmt.Errorf("rpm trigger %d: read trigger script: %w", i+1, err)
		}

		// The rpm library silently drops a trigger with an empty body, while the
		// generated spec would still declare it.
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("rpm trigger %d: script %s is empty", i+1, t.Script)
		}

		conditions := make([]rpm.Relation, 0, len(t.Conditions))
		for _, condition := range t.Conditions {
			// rpmbuild splits trigger conditions on commas.
			if strings.Contains(condition, ",") {
				return nil, fmt.Errorf("rpm trigger %d: condition %q must not contain a comma, use one condition per entry", i+1, condition)
			}

			relation, err := rpm.ParseRelation(condition)
			if err != nil {
				return nil, fmt.Errorf("rpm trigger %d: invalid condition %q: %w", i+1, condition, err)
			}

			// rpmbuild rejects rich dependencies in trigger conditions.
			if strings.HasPrefix(relation.Name(), "(") {
				return nil, fmt.Errorf("rpm trigger %d: rich dependency %q is not allowed in trigger conditions", i+1, condition)
			}

			for _, previous := range defined {
				if previous.triggerType == triggerType && conditionsOverlap(previous.relation, relation) {
					return nil, fmt.Errorf("rpm trigger %d: condition %q overlaps condition %q of rpm trigger %d, which has the same type %q", i+1, condition, previous.condition, previous.trigger, t.Type)
				}
			}
			defined = append(defined, triggerCondition{triggerType, relation, condition, i + 1})

			conditions = append(conditions, relation)
		}

		if len(conditions) == 0 {
			return nil, fmt.Errorf("rpm trigger %d: at least one condition must be provided", i+1)
		}

		interpreter := t.Interpreter
		if interpreter == "" {
			interpreter = "/bin/sh"
		}
		if interpreter != "<lua>" && !strings.HasPrefix(interpreter, "/") {
			return nil, fmt.Errorf("rpm trigger %d: interpreter %q must be an absolute path or <lua>", i+1, interpreter)
		}
		// rpmbuild parses the -p value with popt, which splits on whitespace
		// and interprets quotes and backslashes.
		if strings.ContainsAny(interpreter, " \t\n\v\f\r\"'\\") {
			return nil, fmt.Errorf("rpm trigger %d: interpreter %q must not contain whitespace, quotes or backslashes", i+1, interpreter)
		}

		triggers = append(triggers, trigger{
			triggerType: triggerType,
			directive:   t.Type,
			script:      string(body),
			interpreter: interpreter,
			conditions:  conditions,
		})
	}

	return triggers, nil
}

func parseTriggerType(triggerType string) (rpm.TriggerKind, error) {
	switch triggerType {
	case "prein":
		return rpm.TriggerPrein, nil
	case "in":
		return rpm.TriggerIn, nil
	case "un":
		return rpm.TriggerUn, nil
	case "postun":
		return rpm.TriggerPostun, nil
	default:
		return 0, fmt.Errorf("unknown trigger type %q", triggerType)
	}
}

func applyTriggers(b rpm.PackageBuilder, info *nfpm.Info) error {
	triggers, err := readTriggers(info)
	if err != nil {
		return err
	}

	for _, configured := range triggers {
		builder := b.Trigger(configured.triggerType).
			WithScript(configured.script).
			WithInterpreter(configured.interpreter)
		for _, condition := range configured.conditions {
			builder.On(condition.Name(), condition.Version(), condition.Sense())
		}

		builder.Done()

		// Require the interpreter so that it is installed before the trigger
		// runs.
		if strings.HasPrefix(configured.interpreter, "/") {
			b.Requires().With(configured.interpreter, "", rpm.SenseInterp).Done()
		}
	}

	return nil
}
