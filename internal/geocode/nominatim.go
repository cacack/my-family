package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// NominatimName is the provider name a Nominatim candidate carries.
const NominatimName = "Nominatim"

// nominatimAttribution is the credit a candidate carries when the response
// names no licence of its own: Nominatim serves OpenStreetMap data, under the
// ODbL.
const nominatimAttribution = "Data © OpenStreetMap contributors, ODbL 1.0. https://osm.org/copyright"

// maxResponseBytes bounds the response body Nominatim may send.
const maxResponseBytes = 1 << 20

// Nominatim is a Provider backed by a Nominatim server's /search
// (https://nominatim.org/release-docs/latest/api/Search/). The request
// carries the place name and nothing else from the tree; the User-Agent
// identifies the install by its contact, as the usage policy requires
// (https://operations.osmfoundation.org/policies/nominatim/).
type Nominatim struct {
	searchURL string
	userAgent string
	client    *http.Client
}

// NewNominatim returns a Nominatim provider at baseURL, identified by contact
// (an email address or URL). A nil client means http.DefaultClient; the
// lookup's context bounds each request.
func NewNominatim(baseURL, contact, version string, client *http.Client) *Nominatim {
	if client == nil {
		client = http.DefaultClient
	}
	return &Nominatim{
		searchURL: strings.TrimRight(baseURL, "/") + "/search",
		userAgent: fmt.Sprintf("my-family/%s (+%s)", version, contact),
		client:    client,
	}
}

// nominatimPlace is the part of a jsonv2 search result a candidate uses.
type nominatimPlace struct {
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Licence     string `json:"licence"`
}

// Search implements Provider.
func (n *Nominatim) Search(ctx context.Context, query string, limit int) ([]Candidate, error) {
	params := url.Values{
		"q":      {query},
		"format": {"jsonv2"},
		"limit":  {strconv.Itoa(limit)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.searchURL+"?"+params.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrProvider, err)
	}
	req.Header.Set("User-Agent", n.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return nil, fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrProvider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrProvider, resp.StatusCode)
	}

	var places []nominatimPlace
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&places); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %w", ErrTimeout, err)
		}
		return nil, fmt.Errorf("%w: decode response: %w", ErrProvider, err)
	}
	candidates := make([]Candidate, 0, min(len(places), limit))
	for _, p := range places {
		if len(candidates) == limit {
			break
		}
		lat, latErr := strconv.ParseFloat(p.Lat, 64)
		lon, lonErr := strconv.ParseFloat(p.Lon, 64)
		if latErr != nil || lonErr != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		attribution := p.Licence
		if attribution == "" {
			attribution = nominatimAttribution
		}
		candidates = append(candidates, Candidate{
			DisplayName: p.DisplayName,
			Latitude:    lat,
			Longitude:   lon,
			Query:       query,
			Provider:    NominatimName,
			Attribution: attribution,
		})
	}
	return candidates, nil
}
