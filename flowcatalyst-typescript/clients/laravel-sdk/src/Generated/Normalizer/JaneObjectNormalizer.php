<?php

namespace FlowCatalyst\Generated\Normalizer;

use FlowCatalyst\Generated\Runtime\Normalizer\CheckArray;
use FlowCatalyst\Generated\Runtime\Normalizer\ValidatorTrait;
use Symfony\Component\Serializer\Normalizer\DenormalizerAwareInterface;
use Symfony\Component\Serializer\Normalizer\DenormalizerAwareTrait;
use Symfony\Component\Serializer\Normalizer\DenormalizerInterface;
use Symfony\Component\Serializer\Normalizer\NormalizerAwareInterface;
use Symfony\Component\Serializer\Normalizer\NormalizerAwareTrait;
use Symfony\Component\Serializer\Normalizer\NormalizerInterface;
class JaneObjectNormalizer implements DenormalizerInterface, NormalizerInterface, DenormalizerAwareInterface, NormalizerAwareInterface
{
    use DenormalizerAwareTrait;
    use NormalizerAwareTrait;
    use CheckArray;
    use ValidatorTrait;
    protected $normalizers = [
        
        \FlowCatalyst\Generated\Model\AccessibleClientsResponse::class => \FlowCatalyst\Generated\Normalizer\AccessibleClientsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddNoteRequest::class => \FlowCatalyst\Generated\Normalizer\AddNoteRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddSchemaRequest::class => \FlowCatalyst\Generated\Normalizer\AddSchemaRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddSchemaRequest1::class => \FlowCatalyst\Generated\Normalizer\AddSchemaRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationListResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationListResponse1::class => \FlowCatalyst\Generated\Normalizer\ApplicationListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationRef::class => \FlowCatalyst\Generated\Normalizer\ApplicationRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationResponse1::class => \FlowCatalyst\Generated\Normalizer\ApplicationResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignApplicationAccessRequest::class => \FlowCatalyst\Generated\Normalizer\AssignApplicationAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignRoleRequest::class => \FlowCatalyst\Generated\Normalizer\AssignRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignRolesRequest::class => \FlowCatalyst\Generated\Normalizer\AssignRolesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignRolesRequest1::class => \FlowCatalyst\Generated\Normalizer\AssignRolesRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignRolesRequest2::class => \FlowCatalyst\Generated\Normalizer\AssignRolesRequest2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogDto::class => \FlowCatalyst\Generated\Normalizer\AuditLogDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogListResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchDispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\BatchDispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchEventResponse::class => \FlowCatalyst\Generated\Normalizer\BatchEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientAccessGrantDto::class => \FlowCatalyst\Generated\Normalizer\ClientAccessGrantDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientAccessListResponse::class => \FlowCatalyst\Generated\Normalizer\ClientAccessListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientApplicationDto::class => \FlowCatalyst\Generated\Normalizer\ClientApplicationDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientApplicationsResponse::class => \FlowCatalyst\Generated\Normalizer\ClientApplicationsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientConfigRequest::class => \FlowCatalyst\Generated\Normalizer\ClientConfigRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientDto::class => \FlowCatalyst\Generated\Normalizer\ClientDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientDto1::class => \FlowCatalyst\Generated\Normalizer\ClientDto1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientDto2::class => \FlowCatalyst\Generated\Normalizer\ClientDto2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientInfo::class => \FlowCatalyst\Generated\Normalizer\ClientInfoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientListResponse::class => \FlowCatalyst\Generated\Normalizer\ClientListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientListResponse1::class => \FlowCatalyst\Generated\Normalizer\ClientListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientListResponse2::class => \FlowCatalyst\Generated\Normalizer\ClientListResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientListResponse3::class => \FlowCatalyst\Generated\Normalizer\ClientListResponse3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientResponse::class => \FlowCatalyst\Generated\Normalizer\ClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConfigEntry::class => \FlowCatalyst\Generated\Normalizer\ConfigEntryNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ContextData::class => \FlowCatalyst\Generated\Normalizer\ContextDataNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ContextDataResponse::class => \FlowCatalyst\Generated\Normalizer\ContextDataResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ContextDataResponse1::class => \FlowCatalyst\Generated\Normalizer\ContextDataResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ContextDataResponse2::class => \FlowCatalyst\Generated\Normalizer\ContextDataResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateApplicationRequest::class => \FlowCatalyst\Generated\Normalizer\CreateApplicationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateApplicationRequest1::class => \FlowCatalyst\Generated\Normalizer\CreateApplicationRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateClientRequest::class => \FlowCatalyst\Generated\Normalizer\CreateClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateClientRequest1::class => \FlowCatalyst\Generated\Normalizer\CreateClientRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateClientRequest2::class => \FlowCatalyst\Generated\Normalizer\CreateClientRequest2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateClientResponse::class => \FlowCatalyst\Generated\Normalizer\CreateClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateCorsOriginRequest::class => \FlowCatalyst\Generated\Normalizer\CreateCorsOriginRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateDispatchJobRequest::class => \FlowCatalyst\Generated\Normalizer\CreateDispatchJobRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateDispatchPoolRequest::class => \FlowCatalyst\Generated\Normalizer\CreateDispatchPoolRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEmailDomainMappingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEventRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventResponse::class => \FlowCatalyst\Generated\Normalizer\CreateEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventTypeRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEventTypeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventTypeRequest1::class => \FlowCatalyst\Generated\Normalizer\CreateEventTypeRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateIdentityProviderRequest::class => \FlowCatalyst\Generated\Normalizer\CreateIdentityProviderRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateRoleRequest::class => \FlowCatalyst\Generated\Normalizer\CreateRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateRoleRequest1::class => \FlowCatalyst\Generated\Normalizer\CreateRoleRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateRoleRequest2::class => \FlowCatalyst\Generated\Normalizer\CreateRoleRequest2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateRoleRequest3::class => \FlowCatalyst\Generated\Normalizer\CreateRoleRequest3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateServiceAccountRequest::class => \FlowCatalyst\Generated\Normalizer\CreateServiceAccountRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateServiceAccountResponse::class => \FlowCatalyst\Generated\Normalizer\CreateServiceAccountResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateServicePrincipalRequest::class => \FlowCatalyst\Generated\Normalizer\CreateServicePrincipalRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateSubscriptionRequest::class => \FlowCatalyst\Generated\Normalizer\CreateSubscriptionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateUserRequest::class => \FlowCatalyst\Generated\Normalizer\CreateUserRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateUserRequest1::class => \FlowCatalyst\Generated\Normalizer\CreateUserRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchAttemptResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchAttemptResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchJobReadResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchJobReadResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchPoolDto::class => \FlowCatalyst\Generated\Normalizer\DispatchPoolDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchPoolListResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchPoolListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DomainCheckRequest::class => \FlowCatalyst\Generated\Normalizer\DomainCheckRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DomainCheckResponse::class => \FlowCatalyst\Generated\Normalizer\DomainCheckResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EmailDomainCheckResponse::class => \FlowCatalyst\Generated\Normalizer\EmailDomainCheckResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EmailDomainMappingDto::class => \FlowCatalyst\Generated\Normalizer\EmailDomainMappingDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EmailDomainMappingListResponse::class => \FlowCatalyst\Generated\Normalizer\EmailDomainMappingListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ErrorResponse::class => \FlowCatalyst\Generated\Normalizer\ErrorResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ErrorResponse1::class => \FlowCatalyst\Generated\Normalizer\ErrorResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ErrorResponse2::class => \FlowCatalyst\Generated\Normalizer\ErrorResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ErrorResponse3::class => \FlowCatalyst\Generated\Normalizer\ErrorResponse3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\EventReadResponse::class => \FlowCatalyst\Generated\Normalizer\EventReadResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventResponse::class => \FlowCatalyst\Generated\Normalizer\EventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeBinding::class => \FlowCatalyst\Generated\Normalizer\EventTypeBindingNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeBindingDto::class => \FlowCatalyst\Generated\Normalizer\EventTypeBindingDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeBindingItem::class => \FlowCatalyst\Generated\Normalizer\EventTypeBindingItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeDto::class => \FlowCatalyst\Generated\Normalizer\EventTypeDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeListResponse::class => \FlowCatalyst\Generated\Normalizer\EventTypeListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeListResponse1::class => \FlowCatalyst\Generated\Normalizer\EventTypeListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeResponse::class => \FlowCatalyst\Generated\Normalizer\EventTypeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FeaturesConfig::class => \FlowCatalyst\Generated\Normalizer\FeaturesConfigNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FilterOption::class => \FlowCatalyst\Generated\Normalizer\FilterOptionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FilterOption1::class => \FlowCatalyst\Generated\Normalizer\FilterOption1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\FilterOptionsResponse::class => \FlowCatalyst\Generated\Normalizer\FilterOptionsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FilterOptionsResponse1::class => \FlowCatalyst\Generated\Normalizer\FilterOptionsResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\FilterOptionsResponse2::class => \FlowCatalyst\Generated\Normalizer\FilterOptionsResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantAccessRequest::class => \FlowCatalyst\Generated\Normalizer\GrantAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantClientAccessRequest::class => \FlowCatalyst\Generated\Normalizer\GrantClientAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\HealthResponse::class => \FlowCatalyst\Generated\Normalizer\HealthResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdentityProviderDto::class => \FlowCatalyst\Generated\Normalizer\IdentityProviderDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdentityProviderListResponse::class => \FlowCatalyst\Generated\Normalizer\IdentityProviderListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\JsonValue::class => \FlowCatalyst\Generated\Normalizer\JsonValueNormalizer::class,
        
        \FlowCatalyst\Generated\Model\LoginRequest::class => \FlowCatalyst\Generated\Normalizer\LoginRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\LoginResponse::class => \FlowCatalyst\Generated\Normalizer\LoginResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OAuthCredentials::class => \FlowCatalyst\Generated\Normalizer\OAuthCredentialsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PagedDispatchJobReadResponse::class => \FlowCatalyst\Generated\Normalizer\PagedDispatchJobReadResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PagedDispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\PagedDispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PagedEventResponse::class => \FlowCatalyst\Generated\Normalizer\PagedEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PagedRawDispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\PagedRawDispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PagedRawEventResponse::class => \FlowCatalyst\Generated\Normalizer\PagedRawEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionDto::class => \FlowCatalyst\Generated\Normalizer\PermissionDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionInputDto::class => \FlowCatalyst\Generated\Normalizer\PermissionInputDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionInputDto1::class => \FlowCatalyst\Generated\Normalizer\PermissionInputDto1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionInputDto2::class => \FlowCatalyst\Generated\Normalizer\PermissionInputDto2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionInputDto3::class => \FlowCatalyst\Generated\Normalizer\PermissionInputDto3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionListResponse::class => \FlowCatalyst\Generated\Normalizer\PermissionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PlatformConfigResponse::class => \FlowCatalyst\Generated\Normalizer\PlatformConfigResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PlatformStats::class => \FlowCatalyst\Generated\Normalizer\PlatformStatsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PoolDto::class => \FlowCatalyst\Generated\Normalizer\PoolDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PoolListResponse::class => \FlowCatalyst\Generated\Normalizer\PoolListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalDetailDto::class => \FlowCatalyst\Generated\Normalizer\PrincipalDetailDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalDto::class => \FlowCatalyst\Generated\Normalizer\PrincipalDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalDto1::class => \FlowCatalyst\Generated\Normalizer\PrincipalDto1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalListResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalListResponse1::class => \FlowCatalyst\Generated\Normalizer\PrincipalListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ProcessRequest::class => \FlowCatalyst\Generated\Normalizer\ProcessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ProcessResponse::class => \FlowCatalyst\Generated\Normalizer\ProcessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RawDispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\RawDispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RawEventResponse::class => \FlowCatalyst\Generated\Normalizer\RawEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResetPasswordRequest::class => \FlowCatalyst\Generated\Normalizer\ResetPasswordRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleAssignmentDto::class => \FlowCatalyst\Generated\Normalizer\RoleAssignmentDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleDto::class => \FlowCatalyst\Generated\Normalizer\RoleDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleDto1::class => \FlowCatalyst\Generated\Normalizer\RoleDto1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleDto2::class => \FlowCatalyst\Generated\Normalizer\RoleDto2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleListResponse::class => \FlowCatalyst\Generated\Normalizer\RoleListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleListResponse1::class => \FlowCatalyst\Generated\Normalizer\RoleListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleListResponse2::class => \FlowCatalyst\Generated\Normalizer\RoleListResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleListResponse3::class => \FlowCatalyst\Generated\Normalizer\RoleListResponse3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\RolesAssignedResponse::class => \FlowCatalyst\Generated\Normalizer\RolesAssignedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RotateSecretResponse::class => \FlowCatalyst\Generated\Normalizer\RotateSecretResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountDto::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountListResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SessionUserResponse::class => \FlowCatalyst\Generated\Normalizer\SessionUserResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetConfigRequest::class => \FlowCatalyst\Generated\Normalizer\SetConfigRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SpecVersionResponse::class => \FlowCatalyst\Generated\Normalizer\SpecVersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusChangeRequest::class => \FlowCatalyst\Generated\Normalizer\StatusChangeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusChangeRequest1::class => \FlowCatalyst\Generated\Normalizer\StatusChangeRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusResponse::class => \FlowCatalyst\Generated\Normalizer\StatusResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusResponse1::class => \FlowCatalyst\Generated\Normalizer\StatusResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionDto::class => \FlowCatalyst\Generated\Normalizer\SubscriptionDtoNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionDto1::class => \FlowCatalyst\Generated\Normalizer\SubscriptionDto1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionListResponse::class => \FlowCatalyst\Generated\Normalizer\SubscriptionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionListResponse1::class => \FlowCatalyst\Generated\Normalizer\SubscriptionListResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SwitchClientRequest::class => \FlowCatalyst\Generated\Normalizer\SwitchClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SwitchClientResponse::class => \FlowCatalyst\Generated\Normalizer\SwitchClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncEventTypeItem::class => \FlowCatalyst\Generated\Normalizer\SyncEventTypeItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncEventTypesRequest::class => \FlowCatalyst\Generated\Normalizer\SyncEventTypesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncPoolItem::class => \FlowCatalyst\Generated\Normalizer\SyncPoolItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncPoolsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncPoolsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncResponse::class => \FlowCatalyst\Generated\Normalizer\SyncResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncResponse1::class => \FlowCatalyst\Generated\Normalizer\SyncResponse1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncResponse2::class => \FlowCatalyst\Generated\Normalizer\SyncResponse2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncResponse3::class => \FlowCatalyst\Generated\Normalizer\SyncResponse3Normalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncRoleItem::class => \FlowCatalyst\Generated\Normalizer\SyncRoleItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncRolesRequest::class => \FlowCatalyst\Generated\Normalizer\SyncRolesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncSubscriptionItem::class => \FlowCatalyst\Generated\Normalizer\SyncSubscriptionItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncSubscriptionsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncSubscriptionsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateAccessRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateApplicationRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateApplicationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateApplicationRequest1::class => \FlowCatalyst\Generated\Normalizer\UpdateApplicationRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateAuthTokenRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateAuthTokenRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateClientApplicationsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientRequest1::class => \FlowCatalyst\Generated\Normalizer\UpdateClientRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientRequest2::class => \FlowCatalyst\Generated\Normalizer\UpdateClientRequest2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateDispatchPoolRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateDispatchPoolRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateEmailDomainMappingRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateEmailDomainMappingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateEventTypeRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateEventTypeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateEventTypeRequest1::class => \FlowCatalyst\Generated\Normalizer\UpdateEventTypeRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateIdentityProviderRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdatePrincipalRequest::class => \FlowCatalyst\Generated\Normalizer\UpdatePrincipalRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdatePrincipalRequest1::class => \FlowCatalyst\Generated\Normalizer\UpdatePrincipalRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateRoleRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateRoleRequest1::class => \FlowCatalyst\Generated\Normalizer\UpdateRoleRequest1Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateRoleRequest2::class => \FlowCatalyst\Generated\Normalizer\UpdateRoleRequest2Normalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateServiceAccountRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateServiceAccountRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateSubscriptionRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateSubscriptionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WebhookCredentials::class => \FlowCatalyst\Generated\Normalizer\WebhookCredentialsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OauthTokenPostBody::class => \FlowCatalyst\Generated\Normalizer\OauthTokenPostBodyNormalizer::class,
        
        \Jane\Component\JsonSchemaRuntime\Reference::class => \FlowCatalyst\Generated\Runtime\Normalizer\ReferenceNormalizer::class,
    ], $normalizersCache = [];
    public function supportsDenormalization(mixed $data, string $type, ?string $format = null, array $context = []): bool
    {
        return array_key_exists($type, $this->normalizers);
    }
    public function supportsNormalization(mixed $data, ?string $format = null, array $context = []): bool
    {
        return is_object($data) && array_key_exists(get_class($data), $this->normalizers);
    }
    public function normalize(mixed $data, ?string $format = null, array $context = []): array|string|int|float|bool|\ArrayObject|null
    {
        $normalizerClass = $this->normalizers[get_class($data)];
        $normalizer = $this->getNormalizer($normalizerClass);
        return $normalizer->normalize($data, $format, $context);
    }
    public function denormalize(mixed $data, string $type, ?string $format = null, array $context = []): mixed
    {
        $denormalizerClass = $this->normalizers[$type];
        $denormalizer = $this->getNormalizer($denormalizerClass);
        return $denormalizer->denormalize($data, $type, $format, $context);
    }
    private function getNormalizer(string $normalizerClass)
    {
        return $this->normalizersCache[$normalizerClass] ?? $this->initNormalizer($normalizerClass);
    }
    private function initNormalizer(string $normalizerClass)
    {
        $normalizer = new $normalizerClass();
        $normalizer->setNormalizer($this->normalizer);
        $normalizer->setDenormalizer($this->denormalizer);
        $this->normalizersCache[$normalizerClass] = $normalizer;
        return $normalizer;
    }
    public function getSupportedTypes(?string $format = null): array
    {
        return [
            
            \FlowCatalyst\Generated\Model\AccessibleClientsResponse::class => false,
            \FlowCatalyst\Generated\Model\AddNoteRequest::class => false,
            \FlowCatalyst\Generated\Model\AddSchemaRequest::class => false,
            \FlowCatalyst\Generated\Model\AddSchemaRequest1::class => false,
            \FlowCatalyst\Generated\Model\ApplicationListResponse::class => false,
            \FlowCatalyst\Generated\Model\ApplicationListResponse1::class => false,
            \FlowCatalyst\Generated\Model\ApplicationRef::class => false,
            \FlowCatalyst\Generated\Model\ApplicationResponse::class => false,
            \FlowCatalyst\Generated\Model\ApplicationResponse1::class => false,
            \FlowCatalyst\Generated\Model\AssignApplicationAccessRequest::class => false,
            \FlowCatalyst\Generated\Model\AssignRoleRequest::class => false,
            \FlowCatalyst\Generated\Model\AssignRolesRequest::class => false,
            \FlowCatalyst\Generated\Model\AssignRolesRequest1::class => false,
            \FlowCatalyst\Generated\Model\AssignRolesRequest2::class => false,
            \FlowCatalyst\Generated\Model\AuditLogDto::class => false,
            \FlowCatalyst\Generated\Model\AuditLogListResponse::class => false,
            \FlowCatalyst\Generated\Model\BatchDispatchJobResponse::class => false,
            \FlowCatalyst\Generated\Model\BatchEventResponse::class => false,
            \FlowCatalyst\Generated\Model\ClientAccessGrantDto::class => false,
            \FlowCatalyst\Generated\Model\ClientAccessListResponse::class => false,
            \FlowCatalyst\Generated\Model\ClientApplicationDto::class => false,
            \FlowCatalyst\Generated\Model\ClientApplicationsResponse::class => false,
            \FlowCatalyst\Generated\Model\ClientConfigRequest::class => false,
            \FlowCatalyst\Generated\Model\ClientDto::class => false,
            \FlowCatalyst\Generated\Model\ClientDto1::class => false,
            \FlowCatalyst\Generated\Model\ClientDto2::class => false,
            \FlowCatalyst\Generated\Model\ClientInfo::class => false,
            \FlowCatalyst\Generated\Model\ClientListResponse::class => false,
            \FlowCatalyst\Generated\Model\ClientListResponse1::class => false,
            \FlowCatalyst\Generated\Model\ClientListResponse2::class => false,
            \FlowCatalyst\Generated\Model\ClientListResponse3::class => false,
            \FlowCatalyst\Generated\Model\ClientResponse::class => false,
            \FlowCatalyst\Generated\Model\ConfigEntry::class => false,
            \FlowCatalyst\Generated\Model\ContextData::class => false,
            \FlowCatalyst\Generated\Model\ContextDataResponse::class => false,
            \FlowCatalyst\Generated\Model\ContextDataResponse1::class => false,
            \FlowCatalyst\Generated\Model\ContextDataResponse2::class => false,
            \FlowCatalyst\Generated\Model\CreateApplicationRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateApplicationRequest1::class => false,
            \FlowCatalyst\Generated\Model\CreateClientRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateClientRequest1::class => false,
            \FlowCatalyst\Generated\Model\CreateClientRequest2::class => false,
            \FlowCatalyst\Generated\Model\CreateClientResponse::class => false,
            \FlowCatalyst\Generated\Model\CreateCorsOriginRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateDispatchJobRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateDispatchPoolRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateEventRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateEventResponse::class => false,
            \FlowCatalyst\Generated\Model\CreateEventTypeRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateEventTypeRequest1::class => false,
            \FlowCatalyst\Generated\Model\CreateIdentityProviderRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateRoleRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateRoleRequest1::class => false,
            \FlowCatalyst\Generated\Model\CreateRoleRequest2::class => false,
            \FlowCatalyst\Generated\Model\CreateRoleRequest3::class => false,
            \FlowCatalyst\Generated\Model\CreateServiceAccountRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateServiceAccountResponse::class => false,
            \FlowCatalyst\Generated\Model\CreateServicePrincipalRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateSubscriptionRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateUserRequest::class => false,
            \FlowCatalyst\Generated\Model\CreateUserRequest1::class => false,
            \FlowCatalyst\Generated\Model\DispatchAttemptResponse::class => false,
            \FlowCatalyst\Generated\Model\DispatchJobReadResponse::class => false,
            \FlowCatalyst\Generated\Model\DispatchJobResponse::class => false,
            \FlowCatalyst\Generated\Model\DispatchPoolDto::class => false,
            \FlowCatalyst\Generated\Model\DispatchPoolListResponse::class => false,
            \FlowCatalyst\Generated\Model\DomainCheckRequest::class => false,
            \FlowCatalyst\Generated\Model\DomainCheckResponse::class => false,
            \FlowCatalyst\Generated\Model\EmailDomainCheckResponse::class => false,
            \FlowCatalyst\Generated\Model\EmailDomainMappingDto::class => false,
            \FlowCatalyst\Generated\Model\EmailDomainMappingListResponse::class => false,
            \FlowCatalyst\Generated\Model\ErrorResponse::class => false,
            \FlowCatalyst\Generated\Model\ErrorResponse1::class => false,
            \FlowCatalyst\Generated\Model\ErrorResponse2::class => false,
            \FlowCatalyst\Generated\Model\ErrorResponse3::class => false,
            \FlowCatalyst\Generated\Model\EventReadResponse::class => false,
            \FlowCatalyst\Generated\Model\EventResponse::class => false,
            \FlowCatalyst\Generated\Model\EventTypeBinding::class => false,
            \FlowCatalyst\Generated\Model\EventTypeBindingDto::class => false,
            \FlowCatalyst\Generated\Model\EventTypeBindingItem::class => false,
            \FlowCatalyst\Generated\Model\EventTypeDto::class => false,
            \FlowCatalyst\Generated\Model\EventTypeListResponse::class => false,
            \FlowCatalyst\Generated\Model\EventTypeListResponse1::class => false,
            \FlowCatalyst\Generated\Model\EventTypeResponse::class => false,
            \FlowCatalyst\Generated\Model\FeaturesConfig::class => false,
            \FlowCatalyst\Generated\Model\FilterOption::class => false,
            \FlowCatalyst\Generated\Model\FilterOption1::class => false,
            \FlowCatalyst\Generated\Model\FilterOptionsResponse::class => false,
            \FlowCatalyst\Generated\Model\FilterOptionsResponse1::class => false,
            \FlowCatalyst\Generated\Model\FilterOptionsResponse2::class => false,
            \FlowCatalyst\Generated\Model\GrantAccessRequest::class => false,
            \FlowCatalyst\Generated\Model\GrantClientAccessRequest::class => false,
            \FlowCatalyst\Generated\Model\HealthResponse::class => false,
            \FlowCatalyst\Generated\Model\IdentityProviderDto::class => false,
            \FlowCatalyst\Generated\Model\IdentityProviderListResponse::class => false,
            \FlowCatalyst\Generated\Model\JsonValue::class => false,
            \FlowCatalyst\Generated\Model\LoginRequest::class => false,
            \FlowCatalyst\Generated\Model\LoginResponse::class => false,
            \FlowCatalyst\Generated\Model\OAuthCredentials::class => false,
            \FlowCatalyst\Generated\Model\PagedDispatchJobReadResponse::class => false,
            \FlowCatalyst\Generated\Model\PagedDispatchJobResponse::class => false,
            \FlowCatalyst\Generated\Model\PagedEventResponse::class => false,
            \FlowCatalyst\Generated\Model\PagedRawDispatchJobResponse::class => false,
            \FlowCatalyst\Generated\Model\PagedRawEventResponse::class => false,
            \FlowCatalyst\Generated\Model\PermissionDto::class => false,
            \FlowCatalyst\Generated\Model\PermissionInputDto::class => false,
            \FlowCatalyst\Generated\Model\PermissionInputDto1::class => false,
            \FlowCatalyst\Generated\Model\PermissionInputDto2::class => false,
            \FlowCatalyst\Generated\Model\PermissionInputDto3::class => false,
            \FlowCatalyst\Generated\Model\PermissionListResponse::class => false,
            \FlowCatalyst\Generated\Model\PlatformConfigResponse::class => false,
            \FlowCatalyst\Generated\Model\PlatformStats::class => false,
            \FlowCatalyst\Generated\Model\PoolDto::class => false,
            \FlowCatalyst\Generated\Model\PoolListResponse::class => false,
            \FlowCatalyst\Generated\Model\PrincipalDetailDto::class => false,
            \FlowCatalyst\Generated\Model\PrincipalDto::class => false,
            \FlowCatalyst\Generated\Model\PrincipalDto1::class => false,
            \FlowCatalyst\Generated\Model\PrincipalListResponse::class => false,
            \FlowCatalyst\Generated\Model\PrincipalListResponse1::class => false,
            \FlowCatalyst\Generated\Model\ProcessRequest::class => false,
            \FlowCatalyst\Generated\Model\ProcessResponse::class => false,
            \FlowCatalyst\Generated\Model\RawDispatchJobResponse::class => false,
            \FlowCatalyst\Generated\Model\RawEventResponse::class => false,
            \FlowCatalyst\Generated\Model\ResetPasswordRequest::class => false,
            \FlowCatalyst\Generated\Model\RoleAssignmentDto::class => false,
            \FlowCatalyst\Generated\Model\RoleDto::class => false,
            \FlowCatalyst\Generated\Model\RoleDto1::class => false,
            \FlowCatalyst\Generated\Model\RoleDto2::class => false,
            \FlowCatalyst\Generated\Model\RoleListResponse::class => false,
            \FlowCatalyst\Generated\Model\RoleListResponse1::class => false,
            \FlowCatalyst\Generated\Model\RoleListResponse2::class => false,
            \FlowCatalyst\Generated\Model\RoleListResponse3::class => false,
            \FlowCatalyst\Generated\Model\RolesAssignedResponse::class => false,
            \FlowCatalyst\Generated\Model\RotateSecretResponse::class => false,
            \FlowCatalyst\Generated\Model\ServiceAccountDto::class => false,
            \FlowCatalyst\Generated\Model\ServiceAccountListResponse::class => false,
            \FlowCatalyst\Generated\Model\SessionUserResponse::class => false,
            \FlowCatalyst\Generated\Model\SetConfigRequest::class => false,
            \FlowCatalyst\Generated\Model\SpecVersionResponse::class => false,
            \FlowCatalyst\Generated\Model\StatusChangeRequest::class => false,
            \FlowCatalyst\Generated\Model\StatusChangeRequest1::class => false,
            \FlowCatalyst\Generated\Model\StatusResponse::class => false,
            \FlowCatalyst\Generated\Model\StatusResponse1::class => false,
            \FlowCatalyst\Generated\Model\SubscriptionDto::class => false,
            \FlowCatalyst\Generated\Model\SubscriptionDto1::class => false,
            \FlowCatalyst\Generated\Model\SubscriptionListResponse::class => false,
            \FlowCatalyst\Generated\Model\SubscriptionListResponse1::class => false,
            \FlowCatalyst\Generated\Model\SwitchClientRequest::class => false,
            \FlowCatalyst\Generated\Model\SwitchClientResponse::class => false,
            \FlowCatalyst\Generated\Model\SyncEventTypeItem::class => false,
            \FlowCatalyst\Generated\Model\SyncEventTypesRequest::class => false,
            \FlowCatalyst\Generated\Model\SyncPoolItem::class => false,
            \FlowCatalyst\Generated\Model\SyncPoolsRequest::class => false,
            \FlowCatalyst\Generated\Model\SyncResponse::class => false,
            \FlowCatalyst\Generated\Model\SyncResponse1::class => false,
            \FlowCatalyst\Generated\Model\SyncResponse2::class => false,
            \FlowCatalyst\Generated\Model\SyncResponse3::class => false,
            \FlowCatalyst\Generated\Model\SyncRoleItem::class => false,
            \FlowCatalyst\Generated\Model\SyncRolesRequest::class => false,
            \FlowCatalyst\Generated\Model\SyncSubscriptionItem::class => false,
            \FlowCatalyst\Generated\Model\SyncSubscriptionsRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateAccessRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateApplicationRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateApplicationRequest1::class => false,
            \FlowCatalyst\Generated\Model\UpdateAuthTokenRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateClientRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateClientRequest1::class => false,
            \FlowCatalyst\Generated\Model\UpdateClientRequest2::class => false,
            \FlowCatalyst\Generated\Model\UpdateDispatchPoolRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateEmailDomainMappingRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateEventTypeRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateEventTypeRequest1::class => false,
            \FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdatePrincipalRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdatePrincipalRequest1::class => false,
            \FlowCatalyst\Generated\Model\UpdateRoleRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateRoleRequest1::class => false,
            \FlowCatalyst\Generated\Model\UpdateRoleRequest2::class => false,
            \FlowCatalyst\Generated\Model\UpdateServiceAccountRequest::class => false,
            \FlowCatalyst\Generated\Model\UpdateSubscriptionRequest::class => false,
            \FlowCatalyst\Generated\Model\WebhookCredentials::class => false,
            \FlowCatalyst\Generated\Model\OauthTokenPostBody::class => false,
            \Jane\Component\JsonSchemaRuntime\Reference::class => false,
        ];
    }
}