package mapping

import (
	"encoding/json"
	"os"
	"strings"
)

type JoystickEvent struct {
	Type      string `json:"type"`
	Index     uint8  `json:"index"`
	Value     int16  `json:"value"`
	Ready     bool   `json:"-"`
	Timestamp int64  `json:"-"` // nanosecond timestamp for latency profiling
}

func (j *JoystickEvent) String() string {
	str, _ := json.Marshal(j)
	return string(str)
}

type KeyMode int

const (
	Keyboard KeyMode = iota
	Mouse
)

type KeyMapping struct {
	Code int     `json:"code"`
	Mode KeyMode `json:"mode"`
}

func (k *KeyMapping) String() string {
	str, _ := json.Marshal(k)
	return string(str)
}

type AxisMapping struct {
	PositiveKey []KeyMapping `json:"positive_key"`
	NegativeKey []KeyMapping `json:"negative_key"`
}

type HatMapping struct {
	Up    []KeyMapping `json:"up"`
	Down  []KeyMapping `json:"down"`
	Left  []KeyMapping `json:"left"`
	Right []KeyMapping `json:"right"`
}

type WindowProfileCfg struct {
	NamePattern  string `json:"name,omitempty"`
	ClassPattern string `json:"class,omitempty"`
}

type LayerActivator struct {
	Type      string `json:"type"`      // "button" or "hat"
	Index     uint8  `json:"index"`      // button index or hat index
	Direction string `json:"direction"`  // for hat: "up", "down", "left", "right"
}

type LayerMapping struct {
	Name      string                  `json:"name"`      // e.g., "upper", "lower"
	Activator LayerActivator          `json:"activator"`
	Axes      map[uint8]AxisMapping   `json:"axes,omitempty"`
	Buttons   map[uint8][]KeyMapping   `json:"buttons,omitempty"`
	Hats      map[uint8]HatMapping    `json:"hats,omitempty"`
}

type Mapping struct {
	WindowProfile WindowProfileCfg       `json:"window_profiles"`
	AxisDeadzone  int16                  `json:"axes_deadzone,omitempty"`
	Axes          map[uint8]AxisMapping   `json:"axes,omitempty"`
	Buttons       map[uint8][]KeyMapping `json:"buttons,omitempty"`
	Hats          map[uint8]HatMapping   `json:"hats,omitempty"`
	Layers        []LayerMapping         `json:"layers,omitempty"`
}

func (m *Mapping) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	return nil
}

func (m *Mapping) WriteToFile(path string) error {
	data, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return err
	}

	err = os.WriteFile(path, data, 0755)
	if err != nil {
		return err
	}

	return nil
}

func (m *Mapping) UpdateBinding(keyType, subKey string, index uint8, key []KeyMapping) {
	// Extract layer name if present (e.g., "layer_button:MyLayer" -> keyType="layer_button", layerName="MyLayer")
	layerName := ""
	if parts := strings.Split(keyType, ":"); len(parts) > 1 {
		layerName = parts[1]
		keyType = parts[0]
	}

	switch keyType {
	case "button":
		if m.Buttons == nil {
			m.Buttons = make(map[uint8][]KeyMapping)
		}
		m.Buttons[index] = key

	case "axis":
		if m.Axes == nil {
			m.Axes = make(map[uint8]AxisMapping)
		}
		axis := m.Axes[index]
		switch subKey {
		case "negative":
			axis.NegativeKey = key
		case "positive":
			axis.PositiveKey = key
		}
		m.Axes[index] = axis

	case "hat":
		if m.Hats == nil {
			m.Hats = make(map[uint8]HatMapping)
		}
		hat := m.Hats[index]
		switch subKey {
		case "up":
			hat.Up = key
		case "down":
			hat.Down = key
		case "left":
			hat.Left = key
		case "right":
			hat.Right = key
		}
		m.Hats[index] = hat

	case "layer_button":
		layer := m.getOrCreateLayer(layerName)
		if layer.Buttons == nil {
			layer.Buttons = make(map[uint8][]KeyMapping)
		}
		layer.Buttons[index] = key

	case "layer_axis":
		layer := m.getOrCreateLayer(layerName)
		if layer.Axes == nil {
			layer.Axes = make(map[uint8]AxisMapping)
		}
		axis := layer.Axes[index]
		switch subKey {
		case "negative":
			axis.NegativeKey = key
		case "positive":
			axis.PositiveKey = key
		}
		layer.Axes[index] = axis

	case "layer_hat":
		layer := m.getOrCreateLayer(layerName)
		if layer.Hats == nil {
			layer.Hats = make(map[uint8]HatMapping)
		}
		hat := layer.Hats[index]
		switch subKey {
		case "up":
			hat.Up = key
		case "down":
			hat.Down = key
		case "left":
			hat.Left = key
		case "right":
			hat.Right = key
		}
		layer.Hats[index] = hat
	}
}

func (m *Mapping) getOrCreateLayer(name string) *LayerMapping {
	if m.Layers == nil {
		m.Layers = make([]LayerMapping, 0)
	}

	// Find existing layer by name
	for i := range m.Layers {
		if m.Layers[i].Name == name {
			return &m.Layers[i]
		}
	}

	// Create new layer if not found
	newLayer := LayerMapping{
		Name:      name,
		Activator: LayerActivator{Type: "button", Index: 0},
		Buttons:   make(map[uint8][]KeyMapping),
		Axes:      make(map[uint8]AxisMapping),
		Hats:      make(map[uint8]HatMapping),
	}
	m.Layers = append(m.Layers, newLayer)
	return &m.Layers[len(m.Layers)-1]
}

func (m *Mapping) ClearBindings() {
	key := []KeyMapping{}
	for k := range m.Axes {
		axis := m.Axes[k]
		axis.NegativeKey = key
		axis.PositiveKey = key
		m.Axes[k] = axis
	}
	for k := range m.Buttons {
		m.Buttons[k] = key
	}
	for k := range m.Hats {
		hat := m.Hats[k]
		hat.Up = key
		hat.Down = key
		hat.Left = key
		hat.Right = key
		m.Hats[k] = hat
	}
}
