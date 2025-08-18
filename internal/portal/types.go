package portal

// Dataset represents a dataset from Dresden's OpenData portal
type Dataset struct {
	ID            string            `json:"id"`
	LayerID       string            `json:"layerId,omitempty"`
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	Formats       []string          `json:"formats"`
	DataURLs      map[string]string `json:"dataUrls"`
	SpatialExtent *SpatialExtent    `json:"spatialExtent,omitempty"`
}

// SpatialExtent represents the geographic bounds of a dataset
type SpatialExtent struct {
	BBox []float64 `json:"bbox"`
	CRS  []string  `json:"crs"`
}

// OGCAPIResponse represents a response from OGC API Features
type OGCAPIResponse struct {
	Type           string        `json:"type"`
	Features       []interface{} `json:"features"`
	NumberReturned int           `json:"numberReturned"`
	NumberMatched  int           `json:"numberMatched"`
	Links          []OGCLink     `json:"links,omitempty"`
}

// OGCLink represents a link in OGC API responses
type OGCLink struct {
	Href  string `json:"href"`
	Rel   string `json:"rel"`
	Type  string `json:"type,omitempty"`
	Title string `json:"title,omitempty"`
}

// OGCCollectionsResponse represents the collections endpoint response
type OGCCollectionsResponse struct {
	Links       []OGCLink       `json:"links"`
	Collections []OGCCollection `json:"collections"`
}

// OGCCollection represents a single collection in OGC API
type OGCCollection struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Links       []OGCLink  `json:"links"`
	Extent      *OGCExtent `json:"extent,omitempty"`
	CRS         []string   `json:"crs,omitempty"`
	ItemType    string     `json:"itemType,omitempty"`
}

// OGCExtent represents spatial and temporal extent
type OGCExtent struct {
	Spatial *OGCSpatialExtent `json:"spatial,omitempty"`
}

// OGCSpatialExtent represents spatial bounds
type OGCSpatialExtent struct {
	BBox []float64 `json:"bbox"`
	CRS  string    `json:"crs,omitempty"`
}
