package marathon

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QubitProducts/bamboo/configuration"
)

const (
	externalProxyEnvPrefix = "EXTERNAL_PROXY_"
)

// Describes an app process running
type Task struct {
	Host  string
	Port  int
	Ports []int
}

// An app may have multiple processes
type App struct {
	Id                     string
	EscapedId              string
	HealthCheckPath        string
	Tasks                  []Task
	ServicePort            int
	ServicePorts           []int
	Env                    map[string]string
	HaproxySticky          bool
	HaproxyRedirectToHTTPS bool
	HaproxySSLCertID       string
	HaproxyMode            string
	HaproxyBalance         string
	HaproxyAppDomain       string
	ExternalProxyMap       map[int]string
}

type AppList []App

func (slice AppList) Len() int {
	return len(slice)
}

func (slice AppList) Less(i, j int) bool {
	return slice[i].Id < slice[j].Id
}

func (slice AppList) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (slice AppList) HasSSLCertID() bool {
	for _, app := range slice {
		if app.HaproxySSLCertID != "" {
			return true
		}
	}
	return false
}

func (slice AppList) GetSSLCertIDs() []string {
	certIDs := []string{}
	for _, app := range slice {
		if app.HaproxySSLCertID != "" {
			certIDs = append(certIDs, app.HaproxySSLCertID)
		}
	}
	return certIDs
}

type MarathonTaskList []MarathonTask

type MarathonTasks struct {
	Tasks MarathonTaskList `json:tasks`
}

type MarathonTask struct {
	AppId        string
	Id           string
	Host         string
	Ports        []int
	ServicePorts []int
	StartedAt    string
	StagedAt     string
	Version      string
}

func (slice MarathonTaskList) Len() int {
	return len(slice)
}

func (slice MarathonTaskList) Less(i, j int) bool {
	return slice[i].StagedAt < slice[j].StagedAt
}

func (slice MarathonTaskList) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type MarathonApps struct {
	Apps []MarathonApp `json:apps`
}

type MarathonApp struct {
	Id           string            `json:id`
	HealthChecks []HealthChecks    `json:healthChecks`
	Ports        []int             `json:ports`
	Env          map[string]string `json:env`
}

type HealthChecks struct {
	Path string `json:path`
}

type marathonClient struct {
	httpClient *http.Client
	Endpoints  []string
	Username   string
	Password   string
}

func newMarathonClient(maraconf configuration.Marathon) *marathonClient {
	client := &marathonClient{
		httpClient: http.DefaultClient,
		Endpoints:  maraconf.Endpoints(),
	}
	if maraconf.Username != "" && maraconf.Password != "" {
		client.Username = maraconf.Username
		client.Password = maraconf.Password
	}
	return client
}

func (c *marathonClient) FetchApps() (AppList, error) {
	var applist AppList
	var err error

	// try all configured endpoints until one succeeds
	for _, url := range c.Endpoints {
		applist, err = c._fetchApps(url)
		if err == nil {
			return applist, err
		}
	}

	// return last error
	return nil, err
}

func (c *marathonClient) hasBasicAuth() bool {
	return c.Username != "" && c.Password != ""
}

func (c *marathonClient) _fetchApps(url string) (AppList, error) {
	tasks, err := c.fetchTasks(url)
	if err != nil {
		return nil, err
	}

	marathonApps, err := c.fetchMarathonApps(url)
	if err != nil {
		return nil, err
	}

	apps := createApps(tasks, marathonApps)
	sort.Sort(apps)
	return apps, nil
}

func (c *marathonClient) fetchTasks(endpoint string) (map[string][]MarathonTask, error) {
	req, err := http.NewRequest("GET", endpoint+"/v2/tasks", nil)
	if c.hasBasicAuth() {
		req.SetBasicAuth(c.Username, c.Password)
	}
	req.Header.Add("Accept", "application/json")
	response, err := c.httpClient.Do(req)

	var tasks MarathonTasks

	if err != nil {
		return nil, err
	} else {
		contents, err := ioutil.ReadAll(response.Body)
		defer response.Body.Close()
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(contents, &tasks)
		if err != nil {
			return nil, err
		}

		taskList := tasks.Tasks
		sort.Sort(taskList)

		tasksById := map[string][]MarathonTask{}
		for _, task := range taskList {
			if tasksById[task.AppId] == nil {
				tasksById[task.AppId] = []MarathonTask{}
			}
			tasksById[task.AppId] = append(tasksById[task.AppId], task)
		}

		return tasksById, nil
	}
}

func (c *marathonClient) fetchMarathonApps(endpoint string) (map[string]MarathonApp, error) {
	req, err := http.NewRequest("GET", endpoint+"/v2/apps", nil)
	if c.hasBasicAuth() {
		req.SetBasicAuth(c.Username, c.Password)
	}
	response, err := c.httpClient.Do(req)

	if err != nil {
		return nil, err
	} else {
		defer response.Body.Close()
		var appResponse MarathonApps

		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(contents, &appResponse)
		if err != nil {
			return nil, err
		}

		dataById := map[string]MarathonApp{}

		for _, appConfig := range appResponse.Apps {
			dataById[appConfig.Id] = appConfig
		}

		return dataById, nil
	}
}

func createApps(tasksById map[string][]MarathonTask, marathonApps map[string]MarathonApp) AppList {

	apps := AppList{}

	for appId, tasks := range tasksById {
		simpleTasks := []Task{}

		for _, task := range tasks {
			if len(task.Ports) > 0 {
				simpleTasks = append(simpleTasks, Task{Host: task.Host, Port: task.Ports[0], Ports: task.Ports})
			}
		}

		// Try to handle old app id format without slashes
		appPath := appId
		if !strings.HasPrefix(appId, "/") {
			appPath = "/" + appId
		}

		escapedId := strings.Replace(strings.TrimPrefix(appId, "/"), "/", "-", -1)
		app := App{
			// Since Marathon 0.7, apps are namespaced with path
			Id: appPath,
			// Used for template
			EscapedId:        escapedId,
			Tasks:            simpleTasks,
			HealthCheckPath:  parseHealthCheckPath(marathonApps[appId].HealthChecks),
			Env:              marathonApps[appId].Env,
			HaproxyMode:      "tcp",
			HaproxyBalance:   "roundrobin",
			HaproxyAppDomain: fmt.Sprintf("%s.dkos.io", escapedId),
			ExternalProxyMap: make(map[int]string),
		}

		parseHaproxyEnvs(&app)
		parseExternalProxyEnvs(&app)

		if len(marathonApps[appId].Ports) > 0 {
			app.ServicePort = marathonApps[appId].Ports[0]
			app.ServicePorts = marathonApps[appId].Ports
		}

		apps = append(apps, app)
	}
	return apps
}

func parseHaproxyEnvs(app *App) {
	if value, ok := app.Env["HAPROXY_STICKY"]; ok {
		if strings.ToLower(value) == "true" {
			app.HaproxySticky = true
		}
	}
	if value, ok := app.Env["HAPROXY_REDIRECT_TO_HTTPS"]; ok {
		if strings.ToLower(value) == "true" {
			app.HaproxyRedirectToHTTPS = true
		}
	}
	if value, ok := app.Env["HAPROXY_SSL_CERT_ID"]; ok {
		app.HaproxySSLCertID = value
	}
	if value, ok := app.Env["HAPROXY_MODE"]; ok {
		if value == "tcp" || value == "http" {
			app.HaproxyMode = value
		}
	}
	if value, ok := app.Env["HAPROXY_BALANCE"]; ok {
		app.HaproxyBalance = value
	}
	if value, ok := app.Env["HAPROXY_APP_DOMAIN"]; ok {
		app.HaproxyAppDomain = value
	}
}

func parseExternalProxyEnvs(app *App) {
	for key, value := range app.Env {
		if strings.HasPrefix(key, externalProxyEnvPrefix) {
			portStr := strings.TrimPrefix(key, externalProxyEnvPrefix)
			if port, err := strconv.ParseInt(portStr, 10, 0); err == nil {
				app.ExternalProxyMap[int(port)] = value
			}
		}
	}
}

func parseHealthCheckPath(checks []HealthChecks) string {
	if len(checks) > 0 {
		return checks[0].Path
	}
	return ""
}

/*
	Apps returns a struct that describes Marathon current app and their
	sub tasks information.

	Parameters:
		endpoint: Marathon HTTP endpoint, e.g. http://localhost:8080
*/
func FetchApps(maraconf configuration.Marathon) (AppList, error) {
	client := newMarathonClient(maraconf)
	return client.FetchApps()
}
