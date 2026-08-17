package candidate

import (
	"context"
	"errors"

	"gopkg.in/yaml.v3"
)

type Builder interface {
	Build(context.Context) ([]byte, error)
}

type RuntimeSettings struct {
	Enabled            bool
	MixedPort          int
	AllowLAN           bool
	ExternalController string
	ExternalUI         string
	Secret             string
}

type Overlay struct {
	source   Builder
	settings RuntimeSettings
}

func NewOverlay(source Builder, settings RuntimeSettings) *Overlay {
	return &Overlay{source: source, settings: settings}
}

func (o *Overlay) Build(ctx context.Context) ([]byte, error) {
	candidate, err := o.source.Build(ctx)
	if err != nil {
		return nil, err
	}
	if !o.settings.Enabled {
		return candidate, nil
	}
	var document map[string]any
	if err := yaml.Unmarshal(candidate, &document); err != nil || len(document) == 0 {
		return nil, errors.New("candidate is not a non-empty YAML map")
	}
	document["mixed-port"] = o.settings.MixedPort
	document["allow-lan"] = o.settings.AllowLAN
	document["external-controller"] = o.settings.ExternalController
	if o.settings.ExternalUI != "" {
		document["external-ui"] = o.settings.ExternalUI
	}
	document["secret"] = o.settings.Secret
	result, err := yaml.Marshal(document)
	if err != nil {
		return nil, errors.New("encode runtime settings overlay")
	}
	return result, nil
}
