package updater

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ReleaseForVersion reuses the normal configured release client for one exact
// release selection. Callers still validate update/rollback direction and the
// normal package owner verifies the exact signed payload before handoff.
func (c *Client) ReleaseForVersion(version string) (*Release, error) {
	version = strings.TrimPrefix(version, "v")
	if !updateVersionPattern.MatchString(version) {
		return nil, errors.New("An official running app version is required")
	}
	endpoint, err := url.Parse(c.ReleasesURL)
	if err != nil {
		return nil, err
	}
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/tags/v" + version
	response, err := c.HTTPClient.Get(endpoint.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("The exact running release is not available; recheck normal publication")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, errors.New("Release metadata exceeds bound")
	}
	var release Release
	if json.Unmarshal(raw, &release) != nil || release.TagName != "v"+version || !IsCompleteCommercialRelease(&release) {
		return nil, errors.New("The exact complete running release is not available")
	}
	return &release, nil
}
