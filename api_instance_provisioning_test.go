package brokerapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/drewolson/testflight"
	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/brokerapi/fakes"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Provisioning for the Broker API", func() {

	var fakeServiceBroker *fakes.FakeServiceBroker
	var brokerAPI http.Handler
	var brokerLogger *lagertest.TestLogger
	var credentials = brokerapi.BrokerCredentials{
		Username: "username",
		Password: "password",
	}
	var instanceID = uniqueInstanceID()

	var serviceDetails = brokerapi.ServiceDetails{
		PlanID:           "plan-id",
		OrganizationGUID: "organization-guid",
		SpaceGUID:        "space-guid",
	}

	makeInstanceProvisioningRequest := func(instanceID string, serviceDetails brokerapi.ServiceDetails, queryString string) *testflight.Response {
		response := &testflight.Response{}

		testflight.WithServer(brokerAPI, func(r *testflight.Requester) {
			path := "/v2/service_instances/" + instanceID + queryString

			buffer := &bytes.Buffer{}
			json.NewEncoder(buffer).Encode(serviceDetails)
			request, err := http.NewRequest("PUT", path, buffer)
			Expect(err).NotTo(HaveOccurred())
			request.Header.Add("Content-Type", "application/json")
			request.SetBasicAuth(credentials.Username, credentials.Password)

			response = r.Do(request)
		})
		return response
	}

	makeInstanceProvisioningRequestWithAcceptsIncomplete := func(instanceID string, serviceDetails brokerapi.ServiceDetails, acceptsIncomplete bool) *testflight.Response {
		var acceptsIncompleteFlag string

		if acceptsIncomplete {
			acceptsIncompleteFlag = "?accepts_incomplete=true"
		} else {
			acceptsIncompleteFlag = "?accepts_incomplete=false"
		}

		return makeInstanceProvisioningRequest(instanceID, serviceDetails, acceptsIncompleteFlag)
	}
	Context("Synchronus Provisioning", func() {
		BeforeEach(func() {
			fakeServiceBroker = &fakes.FakeServiceBroker{
				InstanceLimit: 3,
			}
			brokerLogger = lagertest.NewTestLogger("broker-api")
			brokerAPI = brokerapi.New(fakeServiceBroker, brokerLogger, credentials)
		})

		lastLogLine := func() lager.LogFormat {
			noOfLogLines := len(brokerLogger.Logs())
			if noOfLogLines == 0 {
				// better way to raise error?
				err := errors.New("expected some log lines but there were none!")
				Expect(err).NotTo(HaveOccurred())
			}

			return brokerLogger.Logs()[noOfLogLines-1]
		}

		Context("when the accepts_incomplete flag is missing", func() {
			It("defaults to synchronus provisioning", func() {
				makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(fakeServiceBroker.ServiceDetails).To(Equal(serviceDetails))
				Expect(fakeServiceBroker.AcceptsIncomplete).To(Equal(false))
			})
		})

		Context("when client explicitly choses synchronus provisioning", func() {
			It("invokes synchronus provisioning", func() {
				makeInstanceProvisioningRequestWithAcceptsIncomplete(instanceID, serviceDetails, false)
				Expect(fakeServiceBroker.ServiceDetails).To(Equal(serviceDetails))
				Expect(fakeServiceBroker.ProvisionedInstanceIDs).To(ContainElement(instanceID))
			})
		})

		Context("when the instance does not exist", func() {
			It("returns a 201", func() {
				response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(response.StatusCode).To(Equal(201))
			})

			It("returns json with a dashboard_url field", func() {
				response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(response.Body).To(MatchJSON(fixture("provisioning.json")))
			})

			It("invokes Provision on the service broker with all params", func() {
				makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(fakeServiceBroker.ServiceDetails).To(Equal(serviceDetails))
				Expect(fakeServiceBroker.ProvisionedInstanceIDs).To(ContainElement(instanceID))
			})

			Context("when the instance limit has been reached", func() {
				BeforeEach(func() {
					for i := 0; i < fakeServiceBroker.InstanceLimit; i++ {
						makeInstanceProvisioningRequest(uniqueInstanceID(), serviceDetails, "")
					}
				})

				It("returns a 500", func() {
					response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
					Expect(response.StatusCode).To(Equal(500))
				})

				It("returns json with a description field and a useful error message", func() {
					response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
					Expect(response.Body).To(MatchJSON(fixture("instance_limit_error.json")))
				})

				It("logs an appropriate error", func() {
					makeInstanceProvisioningRequest(instanceID, serviceDetails, "")

					Expect(lastLogLine().Message).To(ContainSubstring("provision.instance-limit-reached"))
					Expect(lastLogLine().Data["error"]).To(ContainSubstring("instance limit for this service has been reached"))
				})
			})

			Context("when an unexpected error occurs", func() {
				BeforeEach(func() {
					fakeServiceBroker.ProvisionError = errors.New("broker failed")
				})

				It("returns a 500", func() {
					response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
					Expect(response.StatusCode).To(Equal(500))
				})

				It("returns json with a description field and a useful error message", func() {
					response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
					Expect(response.Body).To(MatchJSON(`{"description":"broker failed"}`))
				})

				It("logs an appropriate error", func() {
					makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
					Expect(lastLogLine().Message).To(ContainSubstring("provision.unknown-error"))
					Expect(lastLogLine().Data["error"]).To(ContainSubstring("broker failed"))
				})
			})

			Context("when we send invalid json", func() {
				makeBadInstanceProvisioningRequest := func(instanceID string) *testflight.Response {
					response := &testflight.Response{}

					testflight.WithServer(brokerAPI, func(r *testflight.Requester) {
						path := "/v2/service_instances/" + instanceID

						body := strings.NewReader("{{{{{")
						request, err := http.NewRequest("PUT", path, body)
						Expect(err).NotTo(HaveOccurred())
						request.Header.Add("Content-Type", "application/json")
						request.SetBasicAuth(credentials.Username, credentials.Password)

						response = r.Do(request)
					})

					return response
				}

				It("returns a 422 bad request", func() {
					response := makeBadInstanceProvisioningRequest(instanceID)
					Expect(response.StatusCode).Should(Equal(422))
				})

				It("logs a message", func() {
					makeBadInstanceProvisioningRequest(instanceID)
					Expect(lastLogLine().Message).To(ContainSubstring("provision.invalid-service-details"))
				})
			})
		})

		Context("when the instance already exists", func() {
			BeforeEach(func() {
				makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
			})

			It("returns a 409", func() {
				response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(response.StatusCode).To(Equal(409))
			})

			It("returns an empty JSON object", func() {
				response := makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(response.Body).To(MatchJSON(`{}`))
			})

			It("logs an appropriate error", func() {
				makeInstanceProvisioningRequest(instanceID, serviceDetails, "")
				Expect(lastLogLine().Message).To(ContainSubstring("provision.instance-already-exists"))
				Expect(lastLogLine().Data["error"]).To(ContainSubstring("instance already exists"))
			})
		})
	})

	Context("Asynchronus Provisioning", func() {

		Context("when the accepts_incomplete flag is true", func() {
			It("calls ProvisionAsync on the service broker", func() {
				acceptsIncomplete := true
				makeInstanceProvisioningRequestWithAcceptsIncomplete(instanceID, serviceDetails, acceptsIncomplete)
				Expect(fakeServiceBroker.ServiceDetails).To(Equal(serviceDetails))

				Expect(fakeServiceBroker.AysncProvisionInstanceIds).To(ContainElement(instanceID))
			})

			Context("when the broker chooses to provision asyncronously", func() {
				BeforeEach(func() {
					fakeServiceBroker = &fakes.FakeServiceBroker{
						InstanceLimit: 3,
					}
					fakeAsyncServiceBroker := &fakes.FakeAsyncServiceBroker{
						*fakeServiceBroker,
					}
					brokerAPI = brokerapi.New(fakeAsyncServiceBroker, brokerLogger, credentials)
				})

				It("returns a 202", func() {
					acceptsIncomplete := true
					response := makeInstanceProvisioningRequestWithAcceptsIncomplete(instanceID, serviceDetails, acceptsIncomplete)
					Expect(response.StatusCode).To(Equal(http.StatusAccepted))
				})
			})

			Context("when the broker chooses to provision syncronously", func() {
				It("returns a 201", func() {
					acceptsIncomplete := false
					response := makeInstanceProvisioningRequestWithAcceptsIncomplete(instanceID, serviceDetails, acceptsIncomplete)
					Expect(response.StatusCode).To(Equal(http.StatusCreated))
				})
			})
		})

		Context("when the accepts_incomplete flag is false", func() {

			Context("when broker can only respond asynchronously", func() {
				BeforeEach(func() {
					fakeServiceBroker = &fakes.FakeServiceBroker{
						InstanceLimit: 3,
					}
					fakeAsyncServiceBroker := &fakes.FakeAsyncOnlyServiceBroker{
						*fakeServiceBroker,
					}
					brokerAPI = brokerapi.New(fakeAsyncServiceBroker, brokerLogger, credentials)
				})

				It("returns a 422", func() {
					acceptsIncomplete := false
					response := makeInstanceProvisioningRequestWithAcceptsIncomplete(instanceID, serviceDetails, acceptsIncomplete)
					Expect(response.StatusCode).To(Equal(422))
					Expect(response.Body).To(MatchJSON(fixture("async_required.json")))
				})
			})
		})

	})
})
