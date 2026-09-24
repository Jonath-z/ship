package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client browses a container registry with the environment's stored
// credentials. Three wire protocols cover the common registries:
//
//   - Docker Hub has its own API (hub.docker.com) and JWT login.
//   - GHCR does not implement the v2 catalog; listing goes through the
//     GitHub packages API with the PAT stored as the registry password.
//   - Everything else speaks the OCI distribution API (/v2/) with Basic
//     auth or a Bearer token challenge.
//
// Only names and tags ever leave this package — credentials are used for
// upstream requests and never echoed into responses or logs.
type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 15 * time.Second}}
}

type Credentials struct {
	Server   string // empty means Docker Hub
	Username string
	Password string
}

const listPageSize = 100

func (client *Client) Repositories(ctx context.Context, credentials Credentials) ([]string, error) {
	switch registryKind(credentials.Server) {
	case kindDockerHub:
		return client.dockerHubRepositories(ctx, credentials)
	case kindGHCR:
		return client.ghcrRepositories(ctx, credentials)
	default:
		return client.v2Repositories(ctx, credentials)
	}
}

func (client *Client) Tags(ctx context.Context, credentials Credentials, repository string) ([]string, error) {
	switch registryKind(credentials.Server) {
	case kindDockerHub:
		return client.dockerHubTags(ctx, credentials, repository)
	case kindGHCR:
		return client.ghcrTags(ctx, credentials, repository)
	default:
		return client.v2Tags(ctx, credentials, repository)
	}
}

type kind int

const (
	kindDockerHub kind = iota
	kindGHCR
	kindV2
)

func registryKind(server string) kind {
	host := strings.ToLower(strings.TrimSuffix(hostOf(server), "/"))
	switch host {
	case "", "docker.io", "index.docker.io", "registry-1.docker.io", "hub.docker.com":
		return kindDockerHub
	case "ghcr.io":
		return kindGHCR
	default:
		return kindV2
	}
}

func hostOf(server string) string {
	trimmed := strings.TrimSpace(server)
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	if index := strings.IndexByte(trimmed, '/'); index >= 0 {
		trimmed = trimmed[:index]
	}
	return trimmed
}

func (client *Client) getJSON(ctx context.Context, target string, header http.Header, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	for name, values := range header {
		request.Header[name] = values
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return upstreamError(response)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(out)
}

func upstreamError(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("registry rejected the stored credentials (%d)", response.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("registry resource not found (%d)", response.StatusCode)
	default:
		return fmt.Errorf("registry request failed (%d)", response.StatusCode)
	}
}

// -- Docker Hub ---------------------------------------------------------------

func (client *Client) dockerHubToken(ctx context.Context, credentials Credentials) (string, error) {
	body := strings.NewReader(fmt.Sprintf(
		`{"username":%q,"password":%q}`, credentials.Username, credentials.Password,
	))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://hub.docker.com/v2/users/login", body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Docker Hub login failed (%d)", response.StatusCode)
	}
	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", err
	}
	return parsed.Token, nil
}

func (client *Client) dockerHubRepositories(ctx context.Context, credentials Credentials) ([]string, error) {
	token, err := client.dockerHubToken(ctx, credentials)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Results []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"results"`
	}
	target := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/?page_size=%d",
		url.PathEscape(credentials.Username), listPageSize)
	header := http.Header{"Authorization": {"Bearer " + token}}
	if err := client.getJSON(ctx, target, header, &parsed); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parsed.Results))
	for _, result := range parsed.Results {
		names = append(names, result.Namespace+"/"+result.Name)
	}
	sort.Strings(names)
	return names, nil
}

func (client *Client) dockerHubTags(ctx context.Context, credentials Credentials, repository string) ([]string, error) {
	token, err := client.dockerHubToken(ctx, credentials)
	if err != nil {
		return nil, err
	}
	namespace, name, found := strings.Cut(repository, "/")
	if !found {
		namespace, name = "library", repository
	}
	var parsed struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	target := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/%s/tags/?page_size=%d",
		url.PathEscape(namespace), url.PathEscape(name), listPageSize)
	header := http.Header{"Authorization": {"Bearer " + token}}
	if err := client.getJSON(ctx, target, header, &parsed); err != nil {
		return nil, err
	}
	tags := make([]string, 0, len(parsed.Results))
	for _, result := range parsed.Results {
		tags = append(tags, result.Name)
	}
	sort.Strings(tags)
	return tags, nil
}

// -- GHCR (GitHub packages API) -------------------------------------------------

func (client *Client) ghcrHeader(credentials Credentials) http.Header {
	return http.Header{
		"Authorization": {"Bearer " + credentials.Password},
		"Accept":        {"application/vnd.github+json"},
	}
}

func (client *Client) ghcrRepositories(ctx context.Context, credentials Credentials) ([]string, error) {
	var parsed []struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	target := fmt.Sprintf(
		"https://api.github.com/user/packages?package_type=container&per_page=%d", listPageSize)
	if err := client.getJSON(ctx, target, client.ghcrHeader(credentials), &parsed); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(parsed))
	for _, item := range parsed {
		names = append(names, strings.ToLower(item.Owner.Login+"/"+item.Name))
	}
	sort.Strings(names)
	return names, nil
}

func (client *Client) ghcrTags(ctx context.Context, credentials Credentials, repository string) ([]string, error) {
	_, packageName, found := strings.Cut(repository, "/")
	if !found {
		packageName = repository
	}
	var parsed []struct {
		Metadata struct {
			Container struct {
				Tags []string `json:"tags"`
			} `json:"container"`
		} `json:"metadata"`
	}
	target := fmt.Sprintf("https://api.github.com/user/packages/container/%s/versions?per_page=%d",
		url.PathEscape(packageName), listPageSize)
	if err := client.getJSON(ctx, target, client.ghcrHeader(credentials), &parsed); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var tags []string
	for _, version := range parsed {
		for _, tag := range version.Metadata.Container.Tags {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	sort.Strings(tags)
	return tags, nil
}

// -- Generic OCI distribution (/v2/) --------------------------------------------

// v2BaseURL keeps the host only: a namespace path in the stored server
// (registry.digitalocean.com/my-registry) is part of repository names in the
// distribution API, not of the API root.
func (client *Client) v2BaseURL(server string) string {
	scheme := "https://"
	if strings.HasPrefix(strings.TrimSpace(server), "http://") {
		scheme = "http://"
	}
	return scheme + hostOf(server)
}

func (client *Client) v2Repositories(ctx context.Context, credentials Credentials) ([]string, error) {
	var parsed struct {
		Repositories []string `json:"repositories"`
	}
	target := fmt.Sprintf("%s/v2/_catalog?n=%d", client.v2BaseURL(credentials.Server), listPageSize)
	if err := client.v2GetJSON(ctx, credentials, target, "registry:catalog:*", &parsed); err != nil {
		return nil, err
	}
	sort.Strings(parsed.Repositories)
	return parsed.Repositories, nil
}

func (client *Client) v2Tags(ctx context.Context, credentials Credentials, repository string) ([]string, error) {
	var parsed struct {
		Tags []string `json:"tags"`
	}
	target := fmt.Sprintf("%s/v2/%s/tags/list?n=%d",
		client.v2BaseURL(credentials.Server), repository, listPageSize)
	scope := "repository:" + repository + ":pull"
	if err := client.v2GetJSON(ctx, credentials, target, scope, &parsed); err != nil {
		return nil, err
	}
	sort.Strings(parsed.Tags)
	return parsed.Tags, nil
}

// v2GetJSON tries Basic auth first and answers a Bearer challenge by fetching
// a token from the advertised realm, as the distribution spec requires.
func (client *Client) v2GetJSON(ctx context.Context, credentials Credentials, target, scope string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(credentials.Username, credentials.Password)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusUnauthorized {
		challenge := response.Header.Get("Www-Authenticate")
		io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		token, err := client.v2Token(ctx, credentials, challenge, scope)
		if err != nil {
			return err
		}
		retry, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		retry.Header.Set("Authorization", "Bearer "+token)
		response, err = client.httpClient.Do(retry)
		if err != nil {
			return err
		}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return upstreamError(response)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(out)
}

func (client *Client) v2Token(ctx context.Context, credentials Credentials, challenge, scope string) (string, error) {
	params := parseBearerChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry requires authentication but sent no Bearer realm")
	}
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("invalid Bearer realm: %w", err)
	}
	query := tokenURL.Query()
	if service := params["service"]; service != "" {
		query.Set("service", service)
	}
	if scope != "" {
		query.Set("scope", scope)
	}
	tokenURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	request.SetBasicAuth(credentials.Username, credentials.Password)
	response, err := client.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry token request failed (%d)", response.StatusCode)
	}
	var parsed struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", err
	}
	if parsed.Token != "" {
		return parsed.Token, nil
	}
	return parsed.AccessToken, nil
}

// parseBearerChallenge extracts key="value" pairs from a WWW-Authenticate
// Bearer header such as: Bearer realm="https://auth.io/token",service="reg".
func parseBearerChallenge(header string) map[string]string {
	params := map[string]string{}
	rest, found := strings.CutPrefix(strings.TrimSpace(header), "Bearer ")
	if !found {
		return params
	}
	for _, part := range strings.Split(rest, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		params[strings.ToLower(key)] = strings.Trim(value, `"`)
	}
	return params
}
