package mapping

import (
	"fmt"
	"strings"
)

type LayerData struct {
	Name        string
	ButtonMap   map[uint8][]KeyMapping
	AxisPos     map[uint8][]KeyMapping
	AxisNeg     map[uint8][]KeyMapping
	HatDir      map[string][]KeyMapping
	ActivatorID string // unique ID like "button_1" or "hat_0_up"
}

type FlatMapping struct {
	ButtonMap    map[uint8][]KeyMapping
	AxisPos      map[uint8][]KeyMapping
	AxisNeg      map[uint8][]KeyMapping
	AxisDeadzone int16
	HatDir       map[string][]KeyMapping
	Layers       []LayerData
	HasLayers    bool // set at compile time, avoids layer checks when false
}

func (m *FlatMapping) Resolve(s *Store, evt JoystickEvent) ([]KeyMapping, []KeyMapping) {
	// Fast path: no layers configured, use simple direct lookup
	if !m.HasLayers {
		return m.resolveSimple(s, evt)
	}

	var pressed, released []KeyMapping

	markPressed := func(keys []KeyMapping) {
		for _, key := range keys {
			pressed = append(pressed, key)
		}
	}

	markReleased := func(keys []KeyMapping) {
		for _, key := range keys {
			released = append(released, key)
		}
	}

	// Check for layer activators and update active layers
	for _, layer := range m.Layers {
		activatorID := layer.ActivatorID
		if activatorID == "" {
			continue
		}

		if len(activatorID) >= 7 && activatorID[:7] == "button_" {
			// Button activator
			if evt.Type == "button" {
				var btnIdx uint8
				if _, err := fmt.Sscanf(activatorID, "button_%d", &btnIdx); err == nil {
					if evt.Index == btnIdx {
						shouldBeActive := evt.Value > 0
						wasActive := s.activeLayers[activatorID]
						s.activeLayers[activatorID] = shouldBeActive
						if wasActive && !shouldBeActive {
							// Layer deactivated - release all keys from this layer
							if layerKeys, ok := s.layerPressedKeys[activatorID]; ok && len(layerKeys) > 0 {
								markReleased(layerKeys)
								s.layerPressedKeys[activatorID] = nil
							}
						}
						// Don't process activator's own mapping - it's just a modifier
						return pressed, released
					}
				}
			}
		} else if len(activatorID) >= 4 && activatorID[:4] == "hat_" {
			// Hat activator - format: "hat_0_up"
			if evt.Type == "hat" {
				var hatIdx uint8
				var dir string
				if _, err := fmt.Sscanf(activatorID, "hat_%d_%s", &hatIdx, &dir); err == nil {
					currDir := dirVal(evt.Value)
					if evt.Index == hatIdx {
						shouldBeActive := (currDir == dir && evt.Value != 0)
						wasActive := s.activeLayers[activatorID]
						s.activeLayers[activatorID] = shouldBeActive
						if wasActive && !shouldBeActive {
							// Layer deactivated - release all keys from this layer
							if layerKeys, ok := s.layerPressedKeys[activatorID]; ok && len(layerKeys) > 0 {
								markReleased(layerKeys)
								s.layerPressedKeys[activatorID] = nil
							}
						}
						// If this hat direction matches the activator, don't process its normal mapping
						if currDir == dir {
							return pressed, released
						}
					}
				}
			}
		}
	}

	// Get list of active layer IDs
	activeLayerIDs := make([]string, 0)
	for activatorID := range s.activeLayers {
		if s.activeLayers[activatorID] {
			activeLayerIDs = append(activeLayerIDs, activatorID)
		}
	}

	// Resolve event using layer mappings if active, otherwise base mappings
	// Check layers in reverse order (last activated takes precedence)
	switch evt.Type {
	case "button":
		var keys []KeyMapping
		var ok bool
		var usedLayerID string

		// Check active layers in reverse order (last activated wins)
		for i := len(activeLayerIDs) - 1; i >= 0; i-- {
			activatorID := activeLayerIDs[i]
			// Find the layer data
			for _, layer := range m.Layers {
				if layer.ActivatorID == activatorID {
					if layerKeys, found := layer.ButtonMap[evt.Index]; found {
						keys = layerKeys
						ok = true
						usedLayerID = activatorID
						break
					}
				}
			}
			if ok {
				break
			}
		}

		// Fall back to base mapping if no layer override
		if !ok {
			keys, ok = m.ButtonMap[evt.Index]
		}

		if ok {
			if evt.Value > 0 {
				markPressed(keys)
				if usedLayerID != "" {
					// Track pressed keys for layer cleanup
					s.layerPressedKeys[usedLayerID] = append(s.layerPressedKeys[usedLayerID], keys...)
				}
			} else {
				markReleased(keys)
				if usedLayerID != "" {
					// Remove from tracked keys
					remaining := make([]KeyMapping, 0)
					for _, k := range s.layerPressedKeys[usedLayerID] {
						found := false
						for _, rk := range keys {
							if k.Code == rk.Code && k.Mode == rk.Mode {
								found = true
								break
							}
						}
						if !found {
							remaining = append(remaining, k)
						}
					}
					s.layerPressedKeys[usedLayerID] = remaining
				}
			}
		}

	case "hat":
		var prevKey, currKey string
		prev := s.lastHat[evt.Index]
		prevKey = key(evt.Index, dirVal(prev))
		curr := evt.Value
		currKey = key(evt.Index, dirVal(curr))

		if prevKey != currKey {
			var prevHatKeys, currHatKeys []KeyMapping
			var prevOk, currOk bool
			var usedLayerID string

			// Check active layers for current direction
			for i := len(activeLayerIDs) - 1; i >= 0; i-- {
				activatorID := activeLayerIDs[i]
				for _, layer := range m.Layers {
					if layer.ActivatorID == activatorID {
						if layerKeys, found := layer.HatDir[currKey]; found {
							currHatKeys = layerKeys
							currOk = true
							usedLayerID = activatorID
							break
						}
						if layerKeys, found := layer.HatDir[prevKey]; found {
							prevHatKeys = layerKeys
							prevOk = true
						}
					}
				}
				if currOk {
					break
				}
			}

			// Fall back to base mapping
			if !currOk {
				currHatKeys, currOk = m.HatDir[currKey]
			}
			if !prevOk {
				prevHatKeys, prevOk = m.HatDir[prevKey]
			}

			if prevOk {
				markReleased(prevHatKeys)
				if usedLayerID != "" {
					// Remove from tracked keys
					remaining := make([]KeyMapping, 0)
					for _, k := range s.layerPressedKeys[usedLayerID] {
						found := false
						for _, rk := range prevHatKeys {
							if k.Code == rk.Code && k.Mode == rk.Mode {
								found = true
								break
							}
						}
						if !found {
							remaining = append(remaining, k)
						}
					}
					s.layerPressedKeys[usedLayerID] = remaining
				}
			}
			if currOk {
				markPressed(currHatKeys)
				if usedLayerID != "" {
					s.layerPressedKeys[usedLayerID] = append(s.layerPressedKeys[usedLayerID], currHatKeys...)
				}
			}
		}

		s.lastHat[evt.Index] = curr

	case "axis":
		var keys []KeyMapping
		var ok bool
		var usedLayerID string

		// Check active layers
		for i := len(activeLayerIDs) - 1; i >= 0; i-- {
			activatorID := activeLayerIDs[i]
			for _, layer := range m.Layers {
				if layer.ActivatorID == activatorID {
					if evt.Value > 0 {
						if layerKeys, found := layer.AxisPos[evt.Index]; found {
							keys = layerKeys
							ok = true
							usedLayerID = activatorID
							break
						}
					} else if evt.Value < 0 {
						if layerKeys, found := layer.AxisNeg[evt.Index]; found {
							keys = layerKeys
							ok = true
							usedLayerID = activatorID
							break
						}
					}
				}
			}
			if ok {
				break
			}
		}

		// Fall back to base mapping
		if !ok {
			keys, ok = m.ResolveAxisKey(evt.Index, evt.Value)
		}

		if ok {
			prev := s.lastAxis[evt.Index]
			var dir int8
			if evt.Value <= -m.AxisDeadzone {
				dir = -1
			} else if evt.Value >= m.AxisDeadzone {
				dir = +1
			}

			// Check if keys contain scroll wheel (needs continuous emit)
			hasScrollWheel := false
			for _, k := range keys {
				if k.Mode == Mouse && (k.Code == 0x001 || k.Code == 0x002) {
					hasScrollWheel = true
					break
				}
			}

			if hasScrollWheel {
				// Scroll wheel: use ticker-based emission for smooth scrolling
				if dir != 0 {
					s.SetScrollActive(evt.Index, keys)
				} else {
					s.SetScrollActive(evt.Index, nil)
				}
			} else if prev != dir {
				if dir != 0 {
					markPressed(keys)
					if usedLayerID != "" {
						s.layerPressedKeys[usedLayerID] = append(s.layerPressedKeys[usedLayerID], keys...)
					}
				} else {
					markReleased(keys)
					if usedLayerID != "" {
						// Remove from tracked keys
						remaining := make([]KeyMapping, 0)
						for _, k := range s.layerPressedKeys[usedLayerID] {
							found := false
							for _, rk := range keys {
								if k.Code == rk.Code && k.Mode == rk.Mode {
									found = true
									break
								}
							}
							if !found {
								remaining = append(remaining, k)
							}
						}
						s.layerPressedKeys[usedLayerID] = remaining
					}
				}
			}

			s.lastAxis[evt.Index] = dir
		}
	}

	return pressed, released
}

// resolveSimple is the fast path when no layers are configured
func (m *FlatMapping) resolveSimple(s *Store, evt JoystickEvent) ([]KeyMapping, []KeyMapping) {
	var pressed, released []KeyMapping

	switch evt.Type {
	case "button":
		if keys, ok := m.ButtonMap[evt.Index]; ok {
			if evt.Value > 0 {
				pressed = append(pressed, keys...)
			} else {
				released = append(released, keys...)
			}
		}

	case "hat":
		prev := s.lastHat[evt.Index]
		prevKey := key(evt.Index, dirVal(prev))
		curr := evt.Value
		currKey := key(evt.Index, dirVal(curr))

		if prevKey != currKey {
			if hatKeys, ok := m.HatDir[prevKey]; ok {
				released = append(released, hatKeys...)
			}
			if hatKeys, ok := m.HatDir[currKey]; ok {
				pressed = append(pressed, hatKeys...)
			}
		}
		s.lastHat[evt.Index] = curr

	case "axis":
		keys, ok := m.ResolveAxisKey(evt.Index, evt.Value)
		if ok {
			prev := s.lastAxis[evt.Index]
			var dir int8
			if evt.Value <= -m.AxisDeadzone {
				dir = -1
			} else if evt.Value >= m.AxisDeadzone {
				dir = +1
			}

			// Check if keys contain scroll wheel (needs continuous emit)
			hasScrollWheel := false
			for _, k := range keys {
				if k.Mode == Mouse && (k.Code == 0x001 || k.Code == 0x002) {
					hasScrollWheel = true
					break
				}
			}

			if hasScrollWheel {
				if dir != 0 {
					s.SetScrollActive(evt.Index, keys)
				} else {
					s.SetScrollActive(evt.Index, nil)
				}
			} else if prev != dir {
				if dir != 0 {
					pressed = append(pressed, keys...)
				} else {
					released = append(released, keys...)
				}
			}
			s.lastAxis[evt.Index] = dir
		}
	}

	return pressed, released
}

func (m *FlatMapping) ResolveAxisKey(axis uint8, value int16) ([]KeyMapping, bool) {
	if value > 0 {
		key, ok := m.AxisPos[axis]
		return key, ok
	} else if value < 0 {
		key, ok := m.AxisNeg[axis]
		return key, ok
	}
	return []KeyMapping{}, false
}

func (m *FlatMapping) GetKeys(keyType, subKey string, index uint8) []KeyMapping {
	var existingKeys []KeyMapping
	
	// Check if keyType includes layer name (format: "layer_button:layer_name")
	layerName := ""
	if parts := strings.Split(keyType, ":"); len(parts) > 1 {
		layerName = parts[1]
		keyType = parts[0]
	}
	
	switch keyType {
	case "axis":
		switch subKey {
		case "positive":
			existingKeys = m.AxisPos[index]
		case "negative":
			existingKeys = m.AxisNeg[index]
		}
	case "hat":
		existingKeys = m.HatDir[key(index, subKey)]
	case "button":
		existingKeys = m.ButtonMap[index]
	case "layer_axis":
		for _, layer := range m.Layers {
			if layer.Name == layerName {
				switch subKey {
				case "positive":
					existingKeys = layer.AxisPos[index]
				case "negative":
					existingKeys = layer.AxisNeg[index]
				}
				break
			}
		}
	case "layer_hat":
		for _, layer := range m.Layers {
			if layer.Name == layerName {
				existingKeys = layer.HatDir[key(index, subKey)]
				break
			}
		}
	case "layer_button":
		for _, layer := range m.Layers {
			if layer.Name == layerName {
				existingKeys = layer.ButtonMap[index]
				break
			}
		}
	}

	return existingKeys
}

func CompileFlatMapping(m Mapping) *FlatMapping {
	f := &FlatMapping{
		ButtonMap:    make(map[uint8][]KeyMapping),
		AxisPos:      make(map[uint8][]KeyMapping),
		AxisNeg:      make(map[uint8][]KeyMapping),
		HatDir:       make(map[string][]KeyMapping),
		AxisDeadzone: m.AxisDeadzone,
		Layers:       make([]LayerData, 0),
		HasLayers:    len(m.Layers) > 0,
	}
	for k, v := range m.Buttons {
		f.ButtonMap[k] = v
	}
	for k, v := range m.Axes {
		f.AxisPos[k] = v.PositiveKey
		f.AxisNeg[k] = v.NegativeKey
	}
	for k, v := range m.Hats {
		f.HatDir[key(k, "up")] = v.Up
		f.HatDir[key(k, "down")] = v.Down
		f.HatDir[key(k, "left")] = v.Left
		f.HatDir[key(k, "right")] = v.Right
	}

	// Compile new Layers array
	for _, layer := range m.Layers {
		layerData := LayerData{
			Name:        layer.Name,
			ButtonMap:   make(map[uint8][]KeyMapping),
			AxisPos:     make(map[uint8][]KeyMapping),
			AxisNeg:     make(map[uint8][]KeyMapping),
			HatDir:      make(map[string][]KeyMapping),
			ActivatorID: "",
		}

		// Set activator ID based on activator type
		if layer.Activator.Type == "button" {
			layerData.ActivatorID = fmt.Sprintf("button_%d", layer.Activator.Index)
		} else if layer.Activator.Type == "hat" {
			layerData.ActivatorID = fmt.Sprintf("hat_%d_%s", layer.Activator.Index, layer.Activator.Direction)
		}

		// Compile layer mappings
		for k, v := range layer.Buttons {
			layerData.ButtonMap[k] = v
		}
		for k, v := range layer.Axes {
			layerData.AxisPos[k] = v.PositiveKey
			layerData.AxisNeg[k] = v.NegativeKey
		}
		for k, v := range layer.Hats {
			layerData.HatDir[key(k, "up")] = v.Up
			layerData.HatDir[key(k, "down")] = v.Down
			layerData.HatDir[key(k, "left")] = v.Left
			layerData.HatDir[key(k, "right")] = v.Right
		}

		f.Layers = append(f.Layers, layerData)
	}

	return f
}

func key(i uint8, dir string) string { return fmt.Sprintf("%d_%s", i, dir) }
func dirVal(i int16) string {
	switch i {
	case 1:
		return "up"
	case 2:
		return "right"
	case 4:
		return "down"
	case 8:
		return "left"
	default:
		return ""
	}
}
