//go:build e2e

package basicrouting

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	serviceManifest               = filepath.Join(fsutils.MustGetThisDir(), "testdata", "service.yaml")
	headlessServiceManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "headless-service.yaml")
	gatewayWithRouteManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-route.yaml")
	longHTTPRouteManifest         = filepath.Join(fsutils.MustGetThisDir(), "testdata", "long-httproute.yaml")
	samePrefixLongGatewayManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gateway-with-same-prefix-80char-names.yaml")

	// test cases
	setup = base.TestCase{
		Manifests: []string{
			gatewayWithRouteManifest,
		},
	}
	testCases = map[string]*base.TestCase{
		"TestGatewayWithRoute": {
			Manifests: []string{serviceManifest},
		},
		"TestHeadlessService": {
			Manifests: []string{headlessServiceManifest},
		},
		"TestLongHTTPRouteName": {
			Manifests: []string{longHTTPRouteManifest},
		},
		"TestSamePrefixLongGatewayNameRouting": {
			Manifests: []string{serviceManifest, samePrefixLongGatewayManifest},
		},
	}

	listenerHighPort = 8080
	listenerLowPort  = 80
)

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
	localGateway common.Gateway
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
		common.Gateway{}, // initialized in SetupSuite
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// Initialize local gateway for this test
	address := s.TestInstallation.Assertions.EventuallyGatewayAddress(
		s.Ctx,
		"gateway",
		"default",
	)
	s.localGateway = common.Gateway{
		NamespacedName: types.NamespacedName{
			Name:      "gateway",
			Namespace: "default",
		},
		Address: address,
	}
}

func (s *testingSuite) TestGatewayWithRoute() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) TestHeadlessService() {
	s.assertSuccessfulResponse()
}

func (s *testingSuite) TestLongHTTPRouteName() {
	longRouteName := "very-long-httproute-name-to-verify-routing-behavior-still-works-much-more-longer"

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		longRouteName,
		"kgateway-base",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		longRouteName,
		"kgateway-base",
		gwv1.RouteConditionResolvedRefs,
		metav1.ConditionTrue,
	)

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithHostHeader("long.example.com"),
		curl.WithPort(80),
	)
}

func (s *testingSuite) TestSamePrefixLongGatewayNameRouting() {
	const (
		gwNameOne    = "very-long-gateway-name-for-testing-80-char-limit-exactly-this-many-chars-aaa-one"
		gwNameTwo    = "very-long-gateway-name-for-testing-80-char-limit-exactly-this-many-chars-bbb-two"
		routeNameOne = "long-gateway-example-route-a"
		routeNameTwo = "long-gateway-example-route-b"
	)

	// Verify the two long names with the same prefix produce different safe names
	s.Require().NotEqual(kubeutils.SafeGatewayLabelValue(gwNameOne), kubeutils.SafeGatewayLabelValue(gwNameTwo))

	// Wait for both proxy pods to be running
	s.TestInstallation.Assertions.EventuallyPodsRunning(
		s.Ctx,
		"default",
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=" + kubeutils.SafeGatewayLabelValue(gwNameOne)},
		time.Second*120,
		time.Millisecond*500,
	)
	s.TestInstallation.Assertions.EventuallyPodsRunning(
		s.Ctx,
		"default",
		metav1.ListOptions{LabelSelector: testdefaults.WellKnownAppLabel + "=" + kubeutils.SafeGatewayLabelValue(gwNameTwo)},
		time.Second*120,
		time.Millisecond*500,
	)

	// Assert both Gateways are programmed
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gwNameOne,
		"default",
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gwNameTwo,
		"default",
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)

	// Assert both HTTPRoutes are accepted
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeNameOne,
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeNameOne,
		"default",
		gwv1.RouteConditionResolvedRefs,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeNameTwo,
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		routeNameTwo,
		"default",
		gwv1.RouteConditionResolvedRefs,
		metav1.ConditionTrue,
	)

	// Get addresses for both Gateways
	firstGateway := common.Gateway{
		NamespacedName: types.NamespacedName{Name: gwNameOne, Namespace: "default"},
		Address:        s.TestInstallation.Assertions.EventuallyGatewayAddress(s.Ctx, gwNameOne, "default"),
	}
	secondGateway := common.Gateway{
		NamespacedName: types.NamespacedName{Name: gwNameTwo, Namespace: "default"},
		Address:        s.TestInstallation.Assertions.EventuallyGatewayAddress(s.Ctx, gwNameTwo, "default"),
	}

	// Verify routing works for both Gateways independently
	firstGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
		curl.WithHostHeader("long-80-a.example.com"),
		curl.WithPort(8080),
	)
	secondGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
		},
		curl.WithHostHeader("long-80-b.example.com"),
		curl.WithPort(8080),
	)
}

func (s *testingSuite) assertSuccessfulResponse() {
	for _, port := range []int{listenerHighPort, listenerLowPort} {
		s.localGateway.Send(
			s.T(),
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(testdefaults.NginxResponse),
			},
			curl.WithHostHeader("example.com"),
			curl.WithPort(port),
		)
	}
}
