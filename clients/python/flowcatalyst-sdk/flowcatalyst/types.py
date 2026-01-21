"""Type definitions for FlowCatalyst SDK"""

from datetime import datetime
from typing import Any, Dict, Literal, Optional
from pydantic import BaseModel


class EventType(BaseModel):
    """Event type definition"""
    id: str
    name: str
    version: str
    schema: Dict[str, Any]
    created_at: datetime


class Subscription(BaseModel):
    """Event subscription"""
    id: str
    event_type_id: str
    endpoint: str
    status: Literal["active", "paused", "failed"]
    created_at: datetime


class DispatchJob(BaseModel):
    """Dispatch job status"""
    id: str
    event_id: str
    subscription_id: str
    status: Literal["pending", "processing", "completed", "failed"]
    attempts: int
    created_at: datetime
    completed_at: Optional[datetime] = None


class ApiResponse(BaseModel):
    """Generic API response wrapper"""
    data: Optional[Any] = None
    error: Optional[str] = None
