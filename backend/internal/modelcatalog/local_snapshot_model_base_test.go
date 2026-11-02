package modelcatalog

import "testing"

func TestLocalChannelModelUsesItsOwnSameOriginEndpoint(t *testing.T) {
	models := ParseLocalChannelModels([]byte(`{"channels":[{"id":"company-relay","baseUrl":"http://10.10.10.45:3000/v1","models":["seedance","image","elsewhere"],"modelProfiles":[
		{"model":"seedance","capability":"video","protocol":"volcengine-ark-video","baseUrl":"http://10.10.10.45:3000/doubao/api/v3"},
		{"model":"image","capability":"image","protocol":"openai-image"},
		{"model":"elsewhere","capability":"video","baseUrl":"http://203.0.113.9/doubao/api/v3"}]}]}`))
	want := map[string]string{"seedance": "http://10.10.10.45:3000/doubao/api/v3", "image": "http://10.10.10.45:3000/v1", "elsewhere": "http://10.10.10.45:3000/v1"}
	for _, item := range models {
		if expected, ok := want[item.Model]; ok && item.BaseURL != expected {
			t.Errorf("%s base = %s, want %s", item.Model, item.BaseURL, expected)
		}
	}
}
