package portal

// Dataset is one entry of the OpenData portal with all its published
// resources, or a feature layer of the city's ArcGIS Online organization
type Dataset struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Updated string `json:"updated,omitempty"`
	Source  string `json:"source,omitempty"`
	License string `json:"license,omitempty"`
	// Portal names where the dataset comes from; empty means the OpenData portal
	Portal  string   `json:"portal,omitempty"`
	Topics  []string `json:"topics,omitempty"`
	Regions []string `json:"regions,omitempty"`
	Years   []string `json:"years,omitempty"`
	// LayerID names the OGC API Features collection of geodata layers
	LayerID   string     `json:"layerId,omitempty"`
	Resources []Resource `json:"resources"`
	// Description and Origin come from the dataset's information page, see Client.Describe
	Description string `json:"description,omitempty"`
	Origin      string `json:"origin,omitempty"`
}

// Resource is one representation of a dataset, e.g. a CSV download or a WMS endpoint
type Resource struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

type SearchResult struct {
	Total    int       `json:"total"`
	Datasets []Dataset `json:"datasets"`
}

// searchRequest is the subset of the portal web app's search request the
// backend requires; it rejects requests missing any of these.
type searchRequest struct {
	TextSearch       string           `json:"textSearch"`
	NumOfResults     int              `json:"numOfResults"`
	PagingStart      int              `json:"pagingStart"`
	UserGroupIDs     []string         `json:"userGroupIds"`
	TextSearchConfig textSearchConfig `json:"textSearchConfig"`
	Result           struct{}         `json:"result"`
}

type textSearchConfig struct {
	Attribute []searchAttribute `json:"attribute"`
}

type searchAttribute struct {
	Name string `json:"name"`
}

type searchResponse struct {
	NumOfResults int            `json:"numOfResults"`
	Results      []searchResult `json:"ipResults"`
}

type searchResult struct {
	Title         string         `json:"bezeichnung"`
	Updated       string         `json:"letzteAenderung"`
	Topics        []string       `json:"themen"`
	Regions       []string       `json:"raeume"`
	Years         []string       `json:"zeiten"`
	DataSource    named          `json:"dataSource"`
	License       named          `json:"presentationLicense"`
	Presentations []presentation `json:"presentations"`
}

type named struct {
	Name string `json:"name"`
}

type presentation struct {
	DatasetID string `json:"ergebnisId"`
	Format    string `json:"darstellungsArtBezeichnung"`
	URL       string `json:"schnittstelle"`
	BaseURL   string `json:"aufrufUrl"`
}
