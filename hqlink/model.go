package hqlink

import (
	"encoding/json"
	"errors"
)

// ErrNotSpatial means the input isn't a BF6 Portal spatial file.
var ErrNotSpatial = errors.New("not a spatial file (no Portal_Dynamic list)")

type hq struct {
	Index       int // position in Portal_Dynamic
	ID, Name    string
	AreaID      string
	Vehicles    []string
	HasVehicles bool // the VehicleSpawners key exists
	VehiclesBad bool // VehicleSpawners exists but isn't a list of strings
}

type spawner struct {
	ID, VehicleType string
	X, Z            float64
}

type spatial struct {
	Objects  []object
	HQs      []hq
	Spawners []spawner
	Polys    map[string][]Point
}

type rawObj struct {
	Type            string                   `json:"type"`
	VehicleType     string                   `json:"VehicleType"`
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	HQArea          string                   `json:"HQArea"`
	VehicleSpawners json.RawMessage          `json:"VehicleSpawners"`
	Position        *struct{ X, Z float64 }  `json:"position"`
	Points          []struct{ X, Z float64 } `json:"points"`
}

// parseSpatial reads HQs, vehicle spawners and polygons from Portal_Dynamic.
// b must not start with a BOM.
func parseSpatial(b []byte) (spatial, error) {
	objs, err := locateDynamic(b)
	if err != nil {
		if errors.Is(err, ErrNotSpatial) {
			return spatial{}, err
		}
		return spatial{}, errors.Join(ErrNotSpatial, err)
	}
	s := spatial{Objects: objs, Polys: map[string][]Point{}}
	for i, o := range objs {
		if o.Open < 0 {
			continue
		}
		var r rawObj
		if err := json.Unmarshal(b[o.Open:o.Close+1], &r); err != nil {
			return spatial{}, errors.Join(ErrNotSpatial, err)
		}
		switch r.Type {
		case "HQ_PlayerSpawner":
			h := hq{Index: i, ID: r.ID, Name: r.Name, AreaID: r.HQArea}
			if _, ok := o.Fields["VehicleSpawners"]; ok {
				h.HasVehicles = true
				if err := json.Unmarshal(r.VehicleSpawners, &h.Vehicles); err != nil {
					h.VehiclesBad = true
				}
			}
			s.HQs = append(s.HQs, h)
		case "VehicleSpawner":
			if r.Position != nil && r.ID != "" {
				s.Spawners = append(s.Spawners, spawner{ID: r.ID, VehicleType: r.VehicleType, X: r.Position.X, Z: r.Position.Z})
			}
		case "PolygonVolume":
			pts := make([]Point, len(r.Points))
			for k, p := range r.Points {
				pts[k] = Point{p.X, p.Z}
			}
			s.Polys[r.ID] = pts
		}
	}
	return s, nil
}
