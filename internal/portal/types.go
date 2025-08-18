package portal

// Dataset represents a dataset from Dresden's OpenData portal
type Dataset struct {
	ID              string            `json:"id"`
	NodeID          string            `json:"nodeId,omitempty"`
	LayerID         string            `json:"layerId,omitempty"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Formats         []string          `json:"formats"`
	MetadataURL     string            `json:"metadataUrl,omitempty"`
	DataURLs        map[string]string `json:"dataUrls"`
	UpdateFrequency string            `json:"updateFrequency,omitempty"`
	License         string            `json:"license"`
	Tags            []string          `json:"tags"`
	SpatialExtent   *SpatialExtent    `json:"spatialExtent,omitempty"`
}

// SpatialExtent represents the geographic bounds of a dataset
type SpatialExtent struct {
	BBox []float64 `json:"bbox"`
	CRS  []string  `json:"crs"`
}

// SearchRequest represents a search request to the portal API
type SearchRequest struct {
	TextSearch     string                 `json:"textSearch"`
	SelectedThemes map[string]interface{} `json:"selectedThemes"`
	Facets         []string               `json:"facets"`
	NumOfResults   int                    `json:"numOfResults"`
	PagingStart    int                    `json:"pagingStart"`
	Sort           string                 `json:"sort"`
}

// SearchResponse represents the response from the portal search API
type SearchResponse struct {
	NumOfHits int                    `json:"numOfHits"`
	Results   []SearchResult         `json:"results"`
	Facets    map[string]interface{} `json:"facets"`
}

// SearchResult represents a single search result
type SearchResult struct {
	ID              string         `json:"nodeId"`
	Bezeichnung     string         `json:"ergebnis_bezeichnung"`
	Beschreibung    string         `json:"titel"`
	Modified        string         `json:"ergebnis_last_modified"`
	Themes          []string       `json:"themen_bezeichnungen"`
	Presentations   []Presentation `json:"presentations"`
	SpatialCoverage []string       `json:"raeume_bezeichnungen"`
}

// Presentation represents a data format/presentation of a dataset
type Presentation struct {
	Type        string `json:"darstellungsart_bezeichnung"`
	URL         string `json:"schnittstelle"`
	Format      string `json:"format,omitempty"`
	Description string `json:"beschreibung,omitempty"`
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
