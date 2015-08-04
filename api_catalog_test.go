package brokerapi_test

import (
	"net/http"

	"github.com/drewolson/testflight"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/brokerapi/fakes"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Catalog endpoint for the broker API", func() {
	var brokerAPI http.Handler
	var fakeServiceBroker *fakes.FakeServiceBroker
	var credentials = brokerapi.BrokerCredentials{
		Username: "username",
		Password: "password",
	}

	makeCatalogRequest := func() *testflight.Response {
		response := &testflight.Response{}
		testflight.WithServer(brokerAPI, func(r *testflight.Requester) {
			request, _ := http.NewRequest("GET", "/v2/catalog", nil)
			request.SetBasicAuth("username", "password")

			response = r.Do(request)
		})
		return response
	}

	BeforeEach(func() {
		fakeServiceBroker = &fakes.FakeServiceBroker{
			InstanceLimit: 3,
		}
		brokerAPI = brokerapi.New(fakeServiceBroker, lagertest.NewTestLogger("broker-api"), credentials)
	})

	It("returns a 200", func() {
		response := makeCatalogRequest()
		Expect(response.StatusCode).To(Equal(200))
	})

	It("returns valid catalog json", func() {
		response := makeCatalogRequest()
		Expect(response.Body).To(MatchJSON(fixture("catalog.json")))
	})
})
