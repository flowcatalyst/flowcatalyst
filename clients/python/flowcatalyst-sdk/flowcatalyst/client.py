"""FlowCatalyst client implementation"""

from typing import List, Optional
import requests
from .types import EventType, Subscription, DispatchJob


class FlowCatalystClient:
    """Client for interacting with FlowCatalyst platform"""

    def __init__(
        self,
        base_url: str,
        api_key: Optional[str] = None,
        timeout: int = 30,
    ):
        """
        Initialize FlowCatalyst client

        Args:
            base_url: Base URL of the FlowCatalyst platform
            api_key: Optional API key for authentication
            timeout: Request timeout in seconds (default: 30)
        """
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.session = requests.Session()

        if api_key:
            self.session.headers.update({"Authorization": f"Bearer {api_key}"})

    def get_event_types(self) -> List[EventType]:
        """Get all event types"""
        response = self._request("GET", "/api/event-types")
        return [EventType(**item) for item in response]

    def get_event_type(self, event_type_id: str) -> EventType:
        """Get a specific event type"""
        response = self._request("GET", f"/api/event-types/{event_type_id}")
        return EventType(**response)

    def create_event_type(
        self, name: str, version: str, schema: dict
    ) -> EventType:
        """Create a new event type"""
        payload = {"name": name, "version": version, "schema": schema}
        response = self._request("POST", "/api/event-types", json=payload)
        return EventType(**response)

    def get_subscriptions(self) -> List[Subscription]:
        """Get all subscriptions"""
        response = self._request("GET", "/api/subscriptions")
        return [Subscription(**item) for item in response]

    def get_subscription(self, subscription_id: str) -> Subscription:
        """Get a specific subscription"""
        response = self._request("GET", f"/api/subscriptions/{subscription_id}")
        return Subscription(**response)

    def create_subscription(
        self, event_type_id: str, endpoint: str, status: str = "active"
    ) -> Subscription:
        """Create a new subscription"""
        payload = {
            "event_type_id": event_type_id,
            "endpoint": endpoint,
            "status": status,
        }
        response = self._request("POST", "/api/subscriptions", json=payload)
        return Subscription(**response)

    def get_dispatch_jobs(self) -> List[DispatchJob]:
        """Get all dispatch jobs"""
        response = self._request("GET", "/api/dispatch-jobs")
        return [DispatchJob(**item) for item in response]

    def get_dispatch_job(self, job_id: str) -> DispatchJob:
        """Get a specific dispatch job"""
        response = self._request("GET", f"/api/dispatch-jobs/{job_id}")
        return DispatchJob(**response)

    def _request(self, method: str, path: str, **kwargs):
        """Internal request method"""
        url = f"{self.base_url}{path}"
        response = self.session.request(
            method, url, timeout=self.timeout, **kwargs
        )
        response.raise_for_status()
        return response.json()
