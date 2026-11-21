package synthsettings

import (
	"context"
	"log/slog"
	"strconv"

	"claude-memory-sync/internal/domain"
	"claude-memory-sync/internal/flows"
	"claude-memory-sync/internal/settings"
)

const maxIntParam = 1000000

type paramDef struct {
	key         string
	def         string
	kind        ParamKind
	description string
}

func paramCatalog() []paramDef {
	return []paramDef{
		{flows.ParamSynthModel, "sonnet", KindModel, "Model used for synthesis and drafting steps."},
		{flows.ParamFidelityModel, "haiku", KindModel, "Model used for cheap fidelity and judgment checks."},
		{flows.ParamStalenessEnabled, "false", KindBool, "Run the staleness review in the daily synthesis flow."},
		{flows.ParamHotcacheMaxBytes, "4000", KindInt, "Maximum size in bytes of the generated hot cache."},
	}
}

func findParam(key string) (paramDef, error) {
	for _, def := range paramCatalog() {
		if def.key == key {
			return def, nil
		}
	}
	return paramDef{}, domain.Error(domain.ErrValidation, "unknown param", slog.String("key", key))
}

func (d paramDef) validate(value string) error {
	valid := false
	switch d.kind {
	case KindModel:
		valid = value == "sonnet" || value == "opus" || value == "haiku"
	case KindBool:
		valid = value == "true" || value == "false"
	case KindInt:
		n, err := strconv.ParseUint(value, 10, 32)
		valid = err == nil && n > 0 && n <= maxIntParam
	}
	if !valid {
		return domain.Error(domain.ErrValidation, "invalid param value", slog.String("key", d.key), slog.String("kind", string(d.kind)))
	}
	return nil
}

func (d paramDef) build(override settings.Setting, overridden bool) Param {
	p := Param{Key: d.key, Value: d.def, Default: d.def, Kind: d.kind, Description: d.description}
	if overridden {
		p.Value = *override.Value
		p.Overridden = true
	}
	return p
}

func (s *Service) Param(ctx context.Context, key string) (string, error) {
	def, err := findParam(key)
	if err != nil {
		return "", err
	}
	override, overridden, err := s.lookup(ctx, paramPrefix+key)
	if err != nil {
		return "", err
	}
	return def.build(override, overridden).Value, nil
}

func (s *Service) Params(ctx context.Context) ([]Param, error) {
	byKey, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	params := make([]Param, 0, len(paramCatalog()))
	for _, def := range paramCatalog() {
		override, overridden := byKey[paramPrefix+def.key]
		params = append(params, def.build(override, overridden))
	}
	return params, nil
}

func (s *Service) SetParam(ctx context.Context, key, value string) (Param, error) {
	def, err := findParam(key)
	if err != nil {
		return Param{}, err
	}
	if err := def.validate(value); err != nil {
		return Param{}, err
	}
	override, err := s.put(ctx, paramPrefix+key, value)
	if err != nil {
		return Param{}, err
	}
	override.Value = &value
	return def.build(override, true), nil
}

func (s *Service) ResetParam(ctx context.Context, key string) (Param, error) {
	def, err := findParam(key)
	if err != nil {
		return Param{}, err
	}
	if err := s.remove(ctx, paramPrefix+key); err != nil {
		return Param{}, err
	}
	return def.build(settings.Setting{}, false), nil
}
