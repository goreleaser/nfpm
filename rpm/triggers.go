package rpm

import (
	"fmt"
	"os"

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

func readTriggers(info *nfpm.Info) ([]trigger, error) {
	triggers := make([]trigger, 0, len(info.RPM.Triggers))
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

		conditions := make([]rpm.Relation, 0, len(t.Conditions))
		for _, condition := range t.Conditions {
			relation, err := rpm.ParseRelation(condition)
			if err != nil {
				return nil, fmt.Errorf("rpm trigger %d: invalid condition %q: %w", i+1, condition, err)
			}

			conditions = append(conditions, relation)
		}

		interpreter := t.Interpreter
		if interpreter == "" {
			interpreter = "/bin/sh"
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
	}

	return nil
}
