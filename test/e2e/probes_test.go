// +build e2e

/*
Copyright 2020 Kohl's Department Stores, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	framework "github.com/operator-framework/operator-sdk/pkg/test"
	util "github.com/operator-framework/operator-sdk/pkg/test/e2eutil"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestReadinessAndLivelinessProbes(t *testing.T) {
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()

	operatorName, found := os.LookupEnv("OPERATOR_NAME")
	if !found {
		operatorName = "eunomia-operator"
	}
	operatorNamespace, found := os.LookupEnv("OPERATOR_NAMESPACE")
	if !found {
		operatorNamespace = "test-eunomia-operator"
	}
	minikubeIP, found := os.LookupEnv("MINIKUBE_IP")
	if !found {
		minikubeIP = "localhost"
	}
	webHookPort, found := os.LookupEnv("WEBHOOK_PORT")
	if !found {
		webHookPort = "8080"
	}
	webHookPortInt, err := strconv.Atoi(webHookPort)
	if err != nil {
		t.Error(err)
	}

	t.Logf("minikube IP: %s", minikubeIP)

	service := &corev1.Service{
		TypeMeta: v1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "minikube-exposing-service",
			Namespace: operatorNamespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "webhook",
					Protocol: corev1.ProtocolTCP,
					Port:     int32(webHookPortInt),
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(webHookPortInt),
					},
				},
			},
			Selector: map[string]string{"name": operatorName},
			Type:     "NodePort",
		},
	}

	err = framework.Global.Client.Create(context.TODO(), service, &framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       timeout,
		RetryInterval: retryInterval,
	})
	if err != nil {
		t.Error(err)
	}
	nodePort := service.Spec.Ports[0].NodePort

	t.Logf("minikube exposing service Node Port: %d", nodePort)

	err = util.WaitForOperatorDeployment(t, framework.Global.KubeClient, operatorNamespace, operatorName, 1, retryInterval, timeout)
	if err != nil {
		t.Error(err)
	}

	//Waiting for service to get connection to operator pod
	maxRetries := 50
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/%s", minikubeIP, nodePort, "readyz"), strings.NewReader(""))
	if err != nil {
		t.Log(err)
	}
	retryCount := 0
	for {
		retryCount++
		t.Logf("retrying %d", retryCount)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Log(err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			break
		}
		if retryCount > maxRetries {
			break
		}
	}

	tests := []struct {
		endpoint      string
		requestBody   string
		requestMethod string
		wantCode      int
		wantBody      string
	}{
		{
			endpoint:      "readyz",
			requestBody:   "",
			requestMethod: http.MethodGet,
			wantCode:      http.StatusOK,
			wantBody:      "ok",
		},
		{
			endpoint:      "healthz",
			requestBody:   "",
			requestMethod: http.MethodGet,
			wantCode:      http.StatusOK,
			wantBody:      "ok",
		},
	}

	for _, tt := range tests {
		req, err := http.NewRequest(tt.requestMethod, fmt.Sprintf("http://%s:%d/%s", minikubeIP, nodePort, tt.endpoint), strings.NewReader(tt.requestBody))
		if err != nil {
			t.Error(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			continue
		}
		defer resp.Body.Close()
		if tt.wantCode != resp.StatusCode {
			t.Errorf("Returned status: %d, wanted: %d", resp.StatusCode, tt.wantCode)
		}
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Error(err)
			continue
		}
		if tt.wantBody != string(bodyBytes) {
			t.Errorf("Returned body: %s, wanted: %s", string(bodyBytes), tt.wantBody)
		}
	}
}
