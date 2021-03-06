package controller

import (
	"testing"
	"time"

	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	argoappv1 "github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	appclientset "github.com/argoproj/argo-cd/pkg/client/clientset/versioned/fake"
	reposerver "github.com/argoproj/argo-cd/reposerver/mocks"
	"github.com/stretchr/testify/assert"
)

func newFakeController(apps ...runtime.Object) *ApplicationController {
	kubeClientset := fake.NewSimpleClientset()
	appClientset := appclientset.NewSimpleClientset(apps...)
	repoClientset := reposerver.Clientset{}
	return NewApplicationController(
		"argocd",
		kubeClientset,
		appClientset,
		&repoClientset,
		time.Minute,
	)
}

var fakeApp = `
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
spec:
  destination:
    namespace: dummy-namespace
    server: https://localhost:6443
  project: default
  source:
    path: some/path
    repoURL: https://github.com/argoproj/argocd-example-apps.git
  syncPolicy:
    automated: {}
status:
  history:
  - deployedAt: 2018-09-08T09:16:50Z
    id: 0
    params: []
    revision: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`

func newFakeApp() *argoappv1.Application {
	var app argoappv1.Application
	err := yaml.Unmarshal([]byte(fakeApp), &app)
	if err != nil {
		panic(err)
	}
	return &app
}

func TestAutoSync(t *testing.T) {
	app := newFakeApp()
	ctrl := newFakeController(app)
	compRes := argoappv1.ComparisonResult{
		Status:   argoappv1.ComparisonStatusOutOfSync,
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	cond := ctrl.autoSync(app, &compRes)
	assert.Nil(t, cond)
	app, err := ctrl.applicationClientset.ArgoprojV1alpha1().Applications("argocd").Get("my-app", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, app.Operation)
	assert.NotNil(t, app.Operation.Sync)
	assert.False(t, app.Operation.Sync.Prune)
}

func TestSkipAutoSync(t *testing.T) {
	// Verify we skip when we previously synced to it in our most recent history
	// Set current to 'aaaaa', desired to 'aaaa' and mark system OutOfSync
	app := newFakeApp()
	ctrl := newFakeController(app)
	compRes := argoappv1.ComparisonResult{
		Status:   argoappv1.ComparisonStatusOutOfSync,
		Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	cond := ctrl.autoSync(app, &compRes)
	assert.Nil(t, cond)
	app, err := ctrl.applicationClientset.ArgoprojV1alpha1().Applications("argocd").Get("my-app", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Nil(t, app.Operation)

	// Verify we skip when we are already Synced (even if revision is different)
	app = newFakeApp()
	ctrl = newFakeController(app)
	compRes = argoappv1.ComparisonResult{
		Status:   argoappv1.ComparisonStatusSynced,
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	cond = ctrl.autoSync(app, &compRes)
	assert.Nil(t, cond)
	app, err = ctrl.applicationClientset.ArgoprojV1alpha1().Applications("argocd").Get("my-app", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Nil(t, app.Operation)

	// Verify we skip when auto-sync is disabled
	app = newFakeApp()
	app.Spec.SyncPolicy = nil
	ctrl = newFakeController(app)
	compRes = argoappv1.ComparisonResult{
		Status:   argoappv1.ComparisonStatusOutOfSync,
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	cond = ctrl.autoSync(app, &compRes)
	assert.Nil(t, cond)
	app, err = ctrl.applicationClientset.ArgoprojV1alpha1().Applications("argocd").Get("my-app", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Nil(t, app.Operation)

	// Verify we skip when previous sync attempt failed and return error condition
	// Set current to 'aaaaa', desired to 'bbbbb' and add 'bbbbb' to failure history
	app = newFakeApp()
	app.Status.OperationState = &argoappv1.OperationState{
		Phase: argoappv1.OperationFailed,
		SyncResult: &argoappv1.SyncOperationResult{
			Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}
	ctrl = newFakeController(app)
	compRes = argoappv1.ComparisonResult{
		Status:   argoappv1.ComparisonStatusOutOfSync,
		Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	cond = ctrl.autoSync(app, &compRes)
	assert.NotNil(t, cond)
	app, err = ctrl.applicationClientset.ArgoprojV1alpha1().Applications("argocd").Get("my-app", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Nil(t, app.Operation)
}
