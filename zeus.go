// Copyright 2015 Cisco Systems, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// 	Unless required by applicable law or agreed to in writing, software
// 	distributed under the License is distributed on an "AS IS" BASIS,
// 	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// 	See the License for the specific language governing permissions and
// 	limitations under the License.

// Package zeusclient provides operations to post and retrieve logs/metrics.
package zeus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Alert contains properties of a alert in a key-value way
type Alert struct {
	Id               int64   `json:"id"`
	Alert_name       string  `json:"alert_name"`
	Created          string  `json:"created"`
	Username         string  `json:"username"`
	Token            string  `json:"token"`
	Alerts_type      string  `json:"alerts_type"`
	Alert_expression string  `json:"alert_expression"`
	Alert_severity   string  `json:"alert_severity"`
	Metric_name      string  `json:"metric_name"`
	Emails           string  `json:"emails"`
	Status           string  `json:"status"`
	Frequency        float64 `json:"frequency"`
	Last_updated     string  `json:"last_updated"`
}

func setAlertToUrlValues(alert Alert, data *url.Values) (errString string) {
	if len(alert.Alert_name) > 0 {
		(*data).Add("alert_name", alert.Alert_name)
	} else {
		return "Alert_name is required"
	}
	if len(alert.Username) > 0 {
		(*data).Add("username", alert.Username)
	}
	if len(alert.Alerts_type) > 0 {
		(*data).Add("alerts_type", alert.Alerts_type)
	}
	if len(alert.Alert_expression) > 0 {
		(*data).Add("alert_expression", alert.Alert_expression)
	} else {
		return "Alert_expression is required"
	}
	if len(alert.Alert_severity) > 0 {
		(*data).Add("alert_severity", alert.Alert_severity)
	}
	if len(alert.Metric_name) > 0 {
		(*data).Add("metric_name", alert.Metric_name)
	}
	if len(alert.Emails) > 0 {
		(*data).Add("emails", alert.Emails)
	}
	if len(alert.Status) > 0 {
		(*data).Add("status", alert.Status)
	}
	if alert.Frequency > 0 {
		(*data).Add("frequency", strconv.FormatFloat(alert.Frequency, 'f', 6, 64))
	}
	return ""
}

// Log contains properties of a log in a key-value way
// Note that Zeus doesn't support nested json, the value has to be string or
// number.
type Log map[string]interface{}

// A collection of logs.
type LogList struct {
	Name string
	Logs []Log
}

func (lst LogList) MarshalJSON() ([]byte, error) {
	return json.Marshal(lst.Logs)
}

// Metric contains two properties of a metric: timestamp of a metric, and
// values of the metric point, which are different dimensions of a data point.
type Metric struct {
	Timestamp float64   `json:"timestamp,omitempty"`
	Point     []float64 `json:"point"`
}

// A collection of metrics.
type MetricList struct {
	Name    string
	Columns []string
	Metrics []Metric
}

func (lst MetricList) MarshalJSON() ([]byte, error) {
	js := []byte("[")
	for i, m := range lst.Metrics {
		if len(m.Point) != len(lst.Columns) {
			return []byte{}, errors.New("field missing")
		}
		p := make(map[string]float64)
		for idx, col := range lst.Columns {
			p[col] = m.Point[idx]
		}
		j, err := json.Marshal(p)
		if err != nil {
			return []byte{}, err
		}
		jstr := `{"point":` + string(j)
		if m.Timestamp != 0 {
			jstr += `, "timestamp":` + strconv.FormatFloat(m.Timestamp, 'f', 3, 64)
		}
		jstr += "}"
		if i != 0 {
			js = append(js, byte(','))
		}
		js = append(js, []byte(jstr)...)
	}
	js = append(js, byte(']'))
	return js, nil
}

func (lst *MetricList) UnmarshalJSON(js []byte) (err error) {
	if len(js) <= 2 {
		return
	}
	var l []map[string]interface{}
	if err := json.Unmarshal(js, &l); err != nil {
		return err
	}
	l0 := l[0]
	if _, ok := l0["name"]; ok == true {
		lst.Name = l0["name"].(string)
	}
	if _, ok := l0["columns"]; ok == true {
		cols := l0["columns"].([]interface{})
		lst.Columns = make([]string, 0, len(cols))
		for _, val := range cols[1:] {
			lst.Columns = append(lst.Columns, val.(string))
		}
	}
	if _, ok := l0["points"]; ok == true {
		points := l0["points"].([]interface{})
		lst.Metrics = make([]Metric, 0, len(points))
		for _, point := range points {
			p := point.([]interface{})
			m := Metric{Timestamp: p[0].(float64)}
			m.Point = make([]float64, 0, len(p)-1)
			for _, val := range p[1:] {
				m.Point = append(m.Point, val.(float64))
			}
			lst.Metrics = append(lst.Metrics, m)
		}
	}
	return
}

// Zeus implements functions to send/receive log, send/receive metrics.
// Constructing Zeus requires URL of Zeus rest api and user token.
type Zeus struct {
	ApiServ, OrganizationAndBucket, Token string
}

type postResponse struct {
	Successful int    `json:"successful"`
	Failed     int    `json:"failed"`
	Error      string `json:"error"`
}

func buildUrl(urls ...string) string {
	return strings.Join(urls, "/") + "/"
}

func (zeus *Zeus) bucket(organization_and_bucket string) *Zeus {
	zeus.OrganizationAndBucket = organization_and_bucket
	return zeus
}

func (zeus *Zeus) request(method, urlStr string, data *url.Values) (
	responseBody []byte, responseStatus int, err error) {
	if zeus.OrganizationAndBucket == "" {
		fmt.Println("Error: Please set bucket's name. ex) zeus.bucket(\"org1/bucket1\").GetLogs()")
		return []byte{}, 0, err
	}
	organizationAndBucket := zeus.OrganizationAndBucket
	zeus.OrganizationAndBucket = ""

	if data == nil {
		data = &url.Values{}
	}
	body := strings.NewReader(data.Encode())

	var request *http.Request
	if method == "GET" {
		request, err = http.NewRequest("GET", urlStr+"?"+data.Encode(), nil)
	} else if method == "POST" {
		request, err = http.NewRequest("POST", urlStr, body)
	} else if method == "PUT" {
		request, err = http.NewRequest("PUT", urlStr, body)
		request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	} else if method == "DELETE" {
		request, err = http.NewRequest("DELETE", urlStr, body)
	}
	if err != nil {
		return []byte{}, 0, err
	}
	request.Header.Add("Authorization", "Bearer "+zeus.Token)
	request.Header.Add("Bucket-Name", organizationAndBucket)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return []byte{}, 0, err
	}

	responseBody, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return []byte{}, 0, err
	}

	responseStatus = response.StatusCode
	response.Body.Close()
	return
}

// PostAlert sends a alert.
// It returns number of successfully sent logs or an error.
func (zeus *Zeus) PostAlert(alert Alert) (successful int, err error) {
	if len(zeus.Token) == 0 {
		return 0, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "alerts", zeus.Token)

	data := make(url.Values)
	errString := setAlertToUrlValues(alert, &data)
	if errString != "" {
		return 0, err
	}

	_, status, err := zeus.request("POST", urlStr, &data)
	if err != nil {
		return 0, err
	}
	if status == 201 {
		successful = 1
	}

	return
}

// GetAlerts returns list of alert
func (zeus *Zeus) GetAlerts() (total int, alerts []Alert, err error) {
	if len(zeus.Token) == 0 {
		return 0, []Alert{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "alerts", zeus.Token)
	data := make(url.Values)

	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return 0, []Alert{}, err
	}

	if status == 200 {
		if err := json.Unmarshal(body, &alerts); err != nil {
			return 0, []Alert{}, err
		}
		total = len(alerts)
	} else if status == 400 {
		return 0, []Alert{}, errors.New("Bad request")
	}
	return
}

// PutAlert update alert which is specified with id
func (zeus *Zeus) PutAlert(id int64, alert Alert) (successful int, err error) {
	if len(zeus.Token) == 0 {
		return 0, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "alerts", zeus.Token, strconv.FormatInt(id, 10))

	data := make(url.Values)
	errString := setAlertToUrlValues(alert, &data)
	if errString != "" {
		return 0, err
	}

	_, status, err := zeus.request("PUT", urlStr, &data)
	if err != nil {
		return 0, err
	}
	if status == 200 {
		successful = 1
	}

	return
}

// GetAlert returns a alert by id
func (zeus *Zeus) GetAlert(id int64) (alert Alert, err error) {
	if len(zeus.Token) == 0 {
		return Alert{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "alerts", zeus.Token, strconv.FormatInt(id, 10))

	data := make(url.Values)
	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return Alert{}, err
	}

	if status == 200 {
		if err := json.Unmarshal(body, &alert); err != nil {
			return Alert{}, err
		}
	} else if status == 400 {
		return Alert{}, errors.New("Bad request")
	}
	return
}

// DeleteAlert delete a alert which is specified with id
func (zeus *Zeus) DeleteAlert(id int64) (successful int, err error) {
	if len(zeus.Token) == 0 {
		return 0, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "alerts", zeus.Token, strconv.FormatInt(id, 10))
	data := make(url.Values)

	_, status, err := zeus.request("DELETE", urlStr, &data)
	if err != nil {
		return 0, err
	}
	if status == 204 {
		successful = 1
	}

	return
}

// GetLogs returns a list of logs that math given constrains. logName is the
// name(or category) of the log. Pattern is a expression to match given log
// field. From and to are the starting and ending timestamp of logs in unix
// time(in seconds). Offest and limit control the pagination of the results.
// If the returned  total larger than the length of return log list, don't
// worry, limit(10 by default) controls the up limit of number of logs
// returned. Please use offset and limit to get the rest logs.
func (zeus *Zeus) GetLogs(logName, field, pattern string, from, to int64,
	offset, limit int) (total int, logs LogList, err error) {
	if len(zeus.Token) == 0 {
		return 0, LogList{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "logs", zeus.Token)
	data := make(url.Values)
	if len(logName) > 0 {
		data.Add("log_name", logName)
	} else {
		return 0, LogList{}, errors.New("logName is required")
	}
	if len(field) > 0 {
		data.Add("attribute_name", field)
	}
	if len(pattern) > 0 {
		data.Add("pattern", pattern)
	}
	if from > 0 {
		data.Add("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		data.Add("to", strconv.FormatInt(to, 10))
	}
	if offset != 0 {
		data.Add("offset", strconv.Itoa(offset))
	}
	if limit != 0 {
		data.Add("limit", strconv.Itoa(limit))
	}

	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return 0, LogList{}, err
	}

	if status == 200 {
		type Resp struct {
			Total  int   `json:"total"`
			Result []Log `json:"result"`
		}
		var resp Resp
		if err := json.Unmarshal(body, &resp); err != nil {
			return 0, LogList{}, err
		}
		total = resp.Total
		logs.Name = logName
		logs.Logs = resp.Result
	} else if status == 400 {
		return 0, LogList{}, errors.New("Bad request")
	}
	return
}

// PostLogs sends a list of logs under given log name. It returns number of
// successfully sent logs or an error.
func (zeus *Zeus) PostLogs(logs LogList) (successful int, err error) {
	if len(logs.Name) == 0 || len(logs.Logs) == 0 {
		return 0, errors.New("logs is empty")
	}
	if len(zeus.Token) == 0 {
		return 0, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "logs", zeus.Token, logs.Name)

	jsonStr, err := json.Marshal(logs)
	if err != nil {
		return 0, err
	}
	data := url.Values{"logs": {string(jsonStr)}}

	body, status, err := zeus.request("POST", urlStr, &data)
	if err != nil {
		return 0, err
	}

	var resp postResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, err
	}
	if status == 200 {
		successful = resp.Successful
	} else {
		return 0, errors.New(resp.Error)
	}
	return
}

// PostMetric sends a list of points under the given metricName.
func (zeus *Zeus) PostMetrics(metrics MetricList) (
	successful int, err error) {
	if len(metrics.Name) == 0 ||
		len(metrics.Columns) == 0 ||
		len(metrics.Metrics) == 0 {
		return 0, errors.New("metrics is empty")
	}
	if len(zeus.Token) == 0 {
		return 0, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "metrics", zeus.Token, metrics.Name)

	jsonStr, err := json.Marshal(metrics)
	if err != nil {
		return 0, err
	}
	data := url.Values{"metrics": {string(jsonStr)}}

	body, status, err := zeus.request("POST", urlStr, &data)
	if err != nil {
		return 0, err
	}
	var resp postResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, err
	}
	if status == 200 {
		successful = resp.Successful
	} else {
		return 0, errors.New(resp.Error)
	}
	return
}

// GetMetricNames returns less than limit of metric names that match regular
// expression metricName.
func (zeus *Zeus) GetMetricNames(metricName string, offset, limit int) (
	names []string, err error) {
	if len(zeus.Token) == 0 {
		return []string{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "metrics", zeus.Token, "_names")
	data := make(url.Values)
	if len(metricName) > 0 {
		data.Add("metric_name", metricName)
	}
	if offset > 0 {
		data.Add("offset", strconv.Itoa(offset))
	}
	if limit > 0 {
		data.Add("limit", strconv.Itoa(limit))
	}

	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return []string{}, err
	}
	if status == 200 {
		if err := json.Unmarshal(body, &names); err != nil {
			return []string{}, err
		}
	}
	return
}

// GetMetricValues returns less than limit of metric values under the name
// metricName, The returned values' timestamp greater than from and smaller
// than to. Values can be aggreated by a function(count, min, max, sum, mean,
// mode, median). Values can also be gouped by a group_interval or filtered by
// filter_condition(value > 0), if value for one field is missing, it'll be
// set to 0.
func (zeus *Zeus) GetMetricValues(metricName string, aggregator string,
	aggregatorCol, groupInterval string, from, to float64, filterCondition string,
	offset, limit int) (metrics MetricList, err error) {
	if len(zeus.Token) == 0 {
		return MetricList{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "metrics", zeus.Token, "_values")
	data := make(url.Values)
	if len(metricName) > 0 {
		data.Add("metric_name", metricName)
	}
	if len(aggregator) > 0 {
		data.Add("aggregator_function", aggregator)
	}
	if len(aggregatorCol) > 0 {
		data.Add("aggregator_column", aggregatorCol)
	}
	if len(groupInterval) > 0 {
		data.Add("group_interval", groupInterval)
	}
	if from > 0 {
		data.Add("from", strconv.FormatFloat(from, 'f', 3, 64))
	}
	if to > 0 {
		data.Add("to", strconv.FormatFloat(to, 'f', 3, 64))
	}
	if len(filterCondition) > 0 {
		data.Add("filter_condition", filterCondition)
	}
	if offset > 0 {
		data.Add("offset", strconv.Itoa(limit))
	}
	if limit > 0 {
		data.Add("limit", strconv.Itoa(limit))
	}

	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return MetricList{}, err
	}
	if status == 200 {
		if err := json.Unmarshal(body, &metrics); err != nil {
			return MetricList{}, err
		}
	}
	return
}

// DeleteMetrics deletes one entire series by the given metric name.
func (zeus *Zeus) DeleteMetrics(metricName string) (bool, error) {
	if len(metricName) == 0 {
		return false, errors.New("metric_name is required")
	}
	if len(zeus.Token) == 0 {
		return false, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "metrics", zeus.Token, metricName)
	data := url.Values{}

	body, status, err := zeus.request("DELETE", urlStr, &data)
	if err != nil {
		return false, err
	}

	if status == 200 {
		var resp []string
		if err := json.Unmarshal(body, &resp); err != nil {
			return false, err
		}
		if resp[0] == "Metric deletion successful" {
			return true, nil
		}
	}
	return false, nil
}

// GetTrigalert returns a trigalert
func (zeus *Zeus) GetTrigalert() (trigalert map[string]interface{}, err error) {
	if len(zeus.Token) == 0 {
		return map[string]interface{}{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "trigalerts", zeus.Token)

	data := make(url.Values)
	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return map[string]interface{}{}, err
	}

	if status == 200 {
		if err := json.Unmarshal(body, &trigalert); err != nil {
			return map[string]interface{}{}, err
		}
	} else if status == 400 {
		return map[string]interface{}{}, errors.New("Bad request")
	}
	return
}

// GetTrigalert returns a trigalert of last24
func (zeus *Zeus) GetTrigalertLast24() (trigalert map[string]interface{}, err error) {
	if len(zeus.Token) == 0 {
		return map[string]interface{}{}, errors.New("API token is empty")
	}
	urlStr := buildUrl(zeus.ApiServ, "trigalerts", zeus.Token, "last24")

	data := make(url.Values)
	body, status, err := zeus.request("GET", urlStr, &data)
	if err != nil {
		return map[string]interface{}{}, err
	}

	if status == 200 {
		if err := json.Unmarshal(body, &trigalert); err != nil {
			return map[string]interface{}{}, err
		}
	} else if status == 400 {
		return map[string]interface{}{}, errors.New("Bad request")
	}
	return
}
