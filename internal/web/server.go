package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/caedis/noreza/internal/input"
	"github.com/caedis/noreza/internal/mapping"
	"github.com/caedis/noreza/internal/web/templates"
)

type rawMapping struct {
	Code string          `json:"code"`
	Mode mapping.KeyMode `json:"mode"`
}

//go:embed static
var staticFiles embed.FS

func RunServer(ctx context.Context, port int, store *mapping.Store, reader *input.Reader, serial string) {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServerFS(staticFiles))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		templates.Layout(reader.String(), serial).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /profiles", func(w http.ResponseWriter, r *http.Request) {
		templates.ProfileList(store.ListProfiles()).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /profiles", func(w http.ResponseWriter, r *http.Request) {
		profileName := r.Header.Get("HX-Prompt")
		profileName = strings.ReplaceAll(profileName, ".", "")
		profileName = strings.ReplaceAll(profileName, ".json", "")
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
			http.Error(w, "profile name missing", http.StatusBadRequest)
			return
		}

		if err := store.CreateNewProfile(fmt.Sprintf("%s.json", profileName)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		templates.ProfileList(store.ListProfiles()).Render(r.Context(), w)
	})

	mux.HandleFunc("DELETE /profiles/{profile}", func(w http.ResponseWriter, r *http.Request) {
		profileName := r.PathValue("profile")

		// delete should trigger store.RemoveProfile
		if err := os.Remove(filepath.Join(store.ProfilePath, profileName+".json")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		templates.EditorDefault(true).Render(r.Context(), w)

		templates.ProfileList(store.ListProfiles()).Render(r.Context(), w)
	})

	mux.HandleFunc("POST /profiles/{profile}/activate", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")
		if profile == "" {
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}

		if err := store.SetActiveProfile(profile); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		templates.ProfileList(store.ListProfiles()).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /profiles/{profile}/update", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")
		vals := r.URL.Query()

		keyType := vals.Get("type")
		subKey := vals.Get("subkey")
		indexVal, _ := strconv.Atoi(vals.Get("index"))
		index := uint8(indexVal)

		mappings := *store.Mappings.Load()
		keyMap, ok := mappings[profile]
		if !ok {
			log.Println("error looking up mapping")
			return
		}

		// Check if this key is used as a layer activator
		layerActivatorName := ""
		baseType := keyType
		if strings.HasPrefix(keyType, "layer_") {
			parts := strings.Split(keyType, ":")
			if len(parts) > 0 {
				baseType = strings.TrimPrefix(parts[0], "layer_")
			}
		}
		rawMappings := *store.RawMappings.Load()
		if rawMapping, ok := rawMappings[profile]; ok {
			for _, layer := range rawMapping.Layers {
				if layer.Activator.Type == baseType && layer.Activator.Index == index {
					if baseType == "hat" && layer.Activator.Direction == subKey {
						layerActivatorName = layer.Name
						break
					} else if baseType == "button" {
						layerActivatorName = layer.Name
						break
					}
				}
			}
		}

		clientKeys := make([]rawMapping, 0)
		existingKeys := keyMap.GetKeys(keyType, subKey, index)
		for _, key := range existingKeys {
			if key.Code == 0 {
				continue
			}

			switch key.Mode {
			case mapping.Mouse:
				clientKeys = append(clientKeys, rawMapping{
					Mode: key.Mode,
					Code: mapping.CodeToMouse[key.Code],
				})
			case mapping.Keyboard:
				clientKeys = append(clientKeys, rawMapping{
					Mode: key.Mode,
					Code: mapping.CodeToKey[key.Code],
				})
			}
		}

		keyString, _ := templ.JSONString(clientKeys)
		templates.EditorModal(profile, index, keyType, subKey, keyString, layerActivatorName).Render(r.Context(), w)
	})

	mux.HandleFunc("PATCH /profiles/{profile}/update", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")

		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updateKeysRaw := r.PostFormValue("updateKeys")

		var rawMap *[]rawMapping
		if err := json.Unmarshal([]byte(updateKeysRaw), &rawMap); err != nil {
			http.Error(w, "unable to parse keys", http.StatusInternalServerError)
			return
		}

		updateKeys := make([]mapping.KeyMapping, 0)
		for _, v := range *rawMap {
			switch v.Mode {
			case mapping.Mouse:
				updateKeys = append(updateKeys, mapping.KeyMapping{
					Mode: v.Mode,
					Code: mapping.MouseToCode[v.Code],
				})
			case mapping.Keyboard:
				updateKeys = append(updateKeys, mapping.KeyMapping{
					Mode: v.Mode,
					Code: mapping.KeyToCode[v.Code],
				})
			}
		}

		keyType := r.PostFormValue("type")
		subKey := r.PostFormValue("subkey")
		index, err := strconv.Atoi(r.PostFormValue("index"))
		if err != nil {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}

		// Update mapping
		mappings := store.RawMappings.Load()
		m, ok := (*mappings)[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		m.UpdateBinding(keyType, subKey, uint8(index), updateKeys)

		// Recompile FlatMapping
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}

		// Update activeMapping if needed
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		if err := store.SaveProfile(profile); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		device, err := mapping.GetDeviceFromID(store.ProductID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Preserve layer mode state
		layerMode := r.PostFormValue("layerMode") == "true"
		selectedLayer := r.PostFormValue("selectedLayer")

		metadata := *store.Metadata.Load()
		templates.Editor(*m, profile, device, metadata, layerMode, selectedLayer).Render(r.Context(), w)
	})

	mux.HandleFunc("GET /profiles/{profile}/editor", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")
		mappings := *store.RawMappings.Load()
		m, ok := mappings[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		device, err := mapping.GetDeviceFromID(store.ProductID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		store.BroadcastEvent(mapping.SSEEvent{
			Type: mapping.EventSelectedProfile,
			Data: profile,
		})

		metadata := *store.Metadata.Load()
		templates.Editor(*m, profile, device, metadata, false, "").Render(r.Context(), w)
	})

	mux.HandleFunc("PATCH /profiles/{profile}/clear", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")

		mappings := store.RawMappings.Load()
		m, ok := (*mappings)[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		m.ClearBindings()

		// Recompile FlatMapping
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}

		// Update activeMapping if needed
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		if err := store.SaveProfile(profile); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		device, err := mapping.GetDeviceFromID(store.ProductID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		metadata := *store.Metadata.Load()
		templates.Editor(*m, profile, device, metadata, false, "").Render(r.Context(), w)
	})

	mux.HandleFunc("GET /profiles/{profile}/settings", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")

		mappings := *store.RawMappings.Load()
		m := mappings[profile]
		windowProfile := m.WindowProfile
		layers := m.Layers
		if layers == nil {
			layers = make([]mapping.LayerMapping, 0)
		}

		templates.SettingsModal(profile, m.AxisDeadzone, windowProfile, layers).Render(r.Context(), w)
	})

	mux.HandleFunc("PATCH /profiles/{profile}/settings/update", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")

		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		namePattern := r.FormValue("nameRegex")
		classPattern := r.FormValue("classRegex")
		deadzoneRaw := r.FormValue("deadzone")
		deadzone, _ := strconv.Atoi(deadzoneRaw)

		mappings := *store.RawMappings.Load()
		m, ok := mappings[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		m.WindowProfile.NamePattern = namePattern
		m.WindowProfile.ClassPattern = classPattern
		m.AxisDeadzone = int16(deadzone)

		// Recompile FlatMapping
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}

		// Update activeMapping if needed
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		path := filepath.Join(store.ProfilePath, profile+".json")
		if err := m.WriteToFile(path); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /profiles/{profile}/layers", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")

		mappings := *store.RawMappings.Load()
		m, ok := mappings[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		if m.Layers == nil {
			m.Layers = make([]mapping.LayerMapping, 0)
		}

		// Add new layer
		newLayer := mapping.LayerMapping{
			Name:      fmt.Sprintf("Layer %d", len(m.Layers)+1),
			Activator: mapping.LayerActivator{Type: "button", Index: 0},
			Buttons:   make(map[uint8][]mapping.KeyMapping),
			Axes:      make(map[uint8]mapping.AxisMapping),
			Hats:      make(map[uint8]mapping.HatMapping),
		}
		m.Layers = append(m.Layers, newLayer)

		// Recompile and save
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		if err := store.SaveProfile(profile); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		// Return the new layer item
		templates.LayerItem(profile, len(m.Layers)-1, newLayer).Render(r.Context(), w)
	})

	mux.HandleFunc("PATCH /profiles/{profile}/layers/{index}", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")
		indexStr := r.PathValue("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mappings := *store.RawMappings.Load()
		m, ok := mappings[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		if m.Layers == nil || index < 0 || index >= len(m.Layers) {
			http.Error(w, "layer not found", http.StatusNotFound)
			return
		}

		layer := &m.Layers[index]

		// Update layer fields
		if name := r.FormValue(fmt.Sprintf("layer_%d_name", index)); name != "" {
			layer.Name = name
		}
		if activatorType := r.FormValue(fmt.Sprintf("layer_%d_activator_type", index)); activatorType != "" {
			layer.Activator.Type = activatorType
		}
		if activatorIndexStr := r.FormValue(fmt.Sprintf("layer_%d_activator_index", index)); activatorIndexStr != "" {
			if idx, err := strconv.Atoi(activatorIndexStr); err == nil {
				layer.Activator.Index = uint8(idx)
			}
		}
		if activatorDir := r.FormValue(fmt.Sprintf("layer_%d_activator_direction", index)); activatorDir != "" {
			layer.Activator.Direction = activatorDir
		}

		// Recompile and save
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		if err := store.SaveProfile(profile); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		// Return updated layer item
		templates.LayerItem(profile, index, *layer).Render(r.Context(), w)
	})

	mux.HandleFunc("DELETE /profiles/{profile}/layers/{index}", func(w http.ResponseWriter, r *http.Request) {
		profile := r.PathValue("profile")
		indexStr := r.PathValue("index")
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			http.Error(w, "invalid index", http.StatusBadRequest)
			return
		}

		mappings := *store.RawMappings.Load()
		m, ok := mappings[profile]
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}

		if m.Layers == nil || index < 0 || index >= len(m.Layers) {
			http.Error(w, "layer not found", http.StatusNotFound)
			return
		}

		// Remove layer
		m.Layers = append(m.Layers[:index], m.Layers[index+1:]...)

		// Recompile and save
		flat := mapping.CompileFlatMapping(*m)
		maps := store.Mappings.Load()
		if maps != nil {
			(*maps)[profile] = flat
			store.Mappings.Store(maps)
		}
		if store.ActiveProfile.Load() == profile {
			store.ActiveMapping.Store(flat)
		}

		if err := store.SaveProfile(profile); err != nil {
			http.Error(w, "error saving profile", http.StatusInternalServerError)
			return
		}

		// Return empty string so htmx removes the element with outerHTML swap
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(""))
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, ": initial keepalive\n\n")
		flusher.Flush()

		ch := store.Subscribe()
		defer store.Unsubscribe(ch)

		clientGone := r.Context().Done()
		keepalive := time.NewTicker(25 * time.Second)
		defer keepalive.Stop()

		fmt.Fprintf(w, "event: activeProfile\n")
		fmt.Fprintf(w, "data: \"%s\"\n\n", store.ActiveProfile.Load())
		flusher.Flush()

		for {
			select {
			case <-keepalive.C:
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			case <-clientGone:
				return
			case evt := <-*ch:
				fmt.Fprintf(w, "event: %s\n", evt.Type)
				data, _ := json.Marshal(evt.Data)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("GET /device/settings", func(w http.ResponseWriter, r *http.Request) {
		metadata := store.Metadata.Load()

		templates.DeviceSettingsModal(*metadata).Render(r.Context(), w)
	})

	mux.HandleFunc("PATCH /device/settings", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		selectedProfile := r.FormValue("selectedProfile")
		selectedProfile = strings.ReplaceAll(selectedProfile, "\"", "")

		oppositeHand := r.FormValue("oppositeHand")
		exclusiveAccess := r.FormValue("exclusiveAccess")
		invertAxes := r.FormValue("invertAxes")

		metadata := mapping.Metadata{
			IsOppositeHand:  oppositeHand == "on",
			ExclusiveAccess: exclusiveAccess == "on",
			InvertAxes: invertAxes == "on",
		}

		store.Metadata.Store(&metadata)
		err := store.SaveMetadata()

		if metadata.ExclusiveAccess {
			reader.Grab()
		} else {
			reader.Ungrab()
		}

		if err != nil {
			http.Error(w, "error saving device settings", http.StatusInternalServerError)
			return
		}
		if selectedProfile != "null" {
			mappings := *store.RawMappings.Load()
			m := mappings[selectedProfile]

			device, err := mapping.GetDeviceFromID(store.ProductID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			templates.Editor(*m, selectedProfile, device, metadata, false, "").Render(r.Context(), w)
		} else {
			templates.EditorDefault(false).Render(r.Context(), w)
		}
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf("localhost:%d", port),
		Handler: mux,
	}
	defer srv.Shutdown(ctx)

	go func() {
		log.Printf("Web server listening on http://localhost:%d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	<-ctx.Done()
	log.Println("Closing web server")
}
