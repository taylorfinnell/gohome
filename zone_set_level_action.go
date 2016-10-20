package gohome

import (
	"fmt"

	"github.com/markdaws/gohome/cmd"
)

type ZoneSetLevelAction struct {
	ZoneID string
	Level  float32
}

func (a *ZoneSetLevelAction) Type() string {
	return "gohome.ZoneSetLevelAction"
}

func (a *ZoneSetLevelAction) Name() string {
	return "Set Zone Level"
}

func (a *ZoneSetLevelAction) Description() string {
	return "Sets the zone level to the specified value"
}

func (a *ZoneSetLevelAction) Ingredients() []Ingredient {
	return []Ingredient{
		Ingredient{
			ID:          "Level",
			Name:        "Intensity Level",
			Description: "The target intensity for the zone",
			Type:        "float",
			Required:    true,
		},
		Ingredient{
			ID:          "ZoneID",
			Name:        "Zone ID",
			Description: "The ID of the target zone",
			Type:        "string",
			Required:    true,
			Reference:   "zone",
		},
	}
}

func (a *ZoneSetLevelAction) Execute(s *System) error {
	zone, ok := s.Zones[a.ZoneID]
	if !ok {
		return fmt.Errorf("Unknown ZoneID %s", a.ZoneID)
	}

	desc := fmt.Sprintf("Zone[%s] Set Level: %f", zone.Name, a.Level)
	return s.CmdProcessor.Enqueue(NewCommandGroup(desc, &cmd.ZoneSetLevel{
		ZoneAddress: zone.Address,
		ZoneID:      zone.ID,
		ZoneName:    zone.Name,
		Level:       cmd.Level{Value: a.Level},
	}))
}

func (a *ZoneSetLevelAction) New() Action {
	return &ZoneSetLevelAction{}
}
