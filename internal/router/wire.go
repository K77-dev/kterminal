package router

import (
	"errors"
	"fmt"

	"kterminal/internal/catalog"
	"kterminal/internal/jev"
	"kterminal/internal/laya"
)

const (
	EngineJev  = "jev"
	EngineLaya = "laya"
)

var ErrEngineUnavailable = errors.New("routing engine not available")

type Options struct {
	Engine     string
	Enabled    bool
	JevAPIKey  string
	JevEnabled bool
	FixedModel string
	Laya       laya.Engine
}

func WireWith(o Options, cat *catalog.Catalog) (Router, Router, error) {
	if !o.Enabled {
		return &FixedRouter{Model: o.FixedModel, Default: cat.DefaultModel}, nil, nil
	}
	switch o.Engine {
	case "", EngineJev:
		if o.JevEnabled && o.JevAPIKey != "" {
			return NewJev(jev.New(o.JevAPIKey)), &HeuristicRouter{Default: cat.DefaultModel}, nil
		}
		return &FixedRouter{Model: o.FixedModel, Default: cat.DefaultModel}, nil, nil
	case EngineLaya:
		if o.Laya == nil {
			return nil, nil, ErrEngineUnavailable
		}
		return &LayaRouter{Engine: o.Laya}, &HeuristicRouter{Default: cat.DefaultModel}, nil
	default:
		return nil, nil, fmt.Errorf("unknown router engine %q", o.Engine)
	}
}
