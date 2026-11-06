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
        
        \FlowCatalyst\Generated\Model\AccessListResponse::class => \FlowCatalyst\Generated\Normalizer\AccessListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AccessResponse::class => \FlowCatalyst\Generated\Normalizer\AccessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddNoteRequest::class => \FlowCatalyst\Generated\Normalizer\AddNoteRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddOriginRequest::class => \FlowCatalyst\Generated\Normalizer\AddOriginRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddRoleRequest::class => \FlowCatalyst\Generated\Normalizer\AddRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AddSchemaRequest::class => \FlowCatalyst\Generated\Normalizer\AddSchemaRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AliasResponse::class => \FlowCatalyst\Generated\Normalizer\AliasResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AllowedOriginResponse::class => \FlowCatalyst\Generated\Normalizer\AllowedOriginResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AnchorDomainListResponse::class => \FlowCatalyst\Generated\Normalizer\AnchorDomainListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AnchorDomainResponse::class => \FlowCatalyst\Generated\Normalizer\AnchorDomainResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AppDocsGroup::class => \FlowCatalyst\Generated\Normalizer\AppDocsGroupNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationAccessListResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationAccessListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationAccessResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationAccessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationFilterListResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationFilterListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationInvitation::class => \FlowCatalyst\Generated\Normalizer\ApplicationInvitationNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationInvitationList::class => \FlowCatalyst\Generated\Normalizer\ApplicationInvitationListNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationListResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationLoginClientCredentials::class => \FlowCatalyst\Generated\Normalizer\ApplicationLoginClientCredentialsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationOAuthClientCredentials::class => \FlowCatalyst\Generated\Normalizer\ApplicationOAuthClientCredentialsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationProvisionLoginClientResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationProvisionLoginClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationProvisionServiceAccountResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationProvisionServiceAccountResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationRolesResponse::class => \FlowCatalyst\Generated\Normalizer\ApplicationRolesResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationServiceAccountCredentials::class => \FlowCatalyst\Generated\Normalizer\ApplicationServiceAccountCredentialsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationUser::class => \FlowCatalyst\Generated\Normalizer\ApplicationUserNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplicationUserList::class => \FlowCatalyst\Generated\Normalizer\ApplicationUserListNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApplyIngestionSetRequest::class => \FlowCatalyst\Generated\Normalizer\ApplyIngestionSetRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignApplicationAccessRequest::class => \FlowCatalyst\Generated\Normalizer\AssignApplicationAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignPrincipalRolesRequest::class => \FlowCatalyst\Generated\Normalizer\AssignPrincipalRolesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignRolesRequest::class => \FlowCatalyst\Generated\Normalizer\AssignRolesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignUnassignedBody::class => \FlowCatalyst\Generated\Normalizer\AssignUnassignedBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AssignUnassignedResponse::class => \FlowCatalyst\Generated\Normalizer\AssignUnassignedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AttachServiceAccountRequest::class => \FlowCatalyst\Generated\Normalizer\AttachServiceAccountRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AttemptDTO::class => \FlowCatalyst\Generated\Normalizer\AttemptDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogApplicationIDsResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogApplicationIDsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogClientIDsResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogClientIDsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogEntityTypesResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogEntityTypesResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogListResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogOperationsResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogOperationsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuditLogResponse::class => \FlowCatalyst\Generated\Normalizer\AuditLogResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuthConfigListResponse::class => \FlowCatalyst\Generated\Normalizer\AuthConfigListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuthConfigResponse::class => \FlowCatalyst\Generated\Normalizer\AuthConfigResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuthenticateBeginRequest::class => \FlowCatalyst\Generated\Normalizer\AuthenticateBeginRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuthenticateBeginResponse::class => \FlowCatalyst\Generated\Normalizer\AuthenticateBeginResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\AuthenticateCompleteRequest::class => \FlowCatalyst\Generated\Normalizer\AuthenticateCompleteRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchEventItem::class => \FlowCatalyst\Generated\Normalizer\BatchEventItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchRequest::class => \FlowCatalyst\Generated\Normalizer\BatchRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchResponse::class => \FlowCatalyst\Generated\Normalizer\BatchResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BatchResultItem::class => \FlowCatalyst\Generated\Normalizer\BatchResultItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BudgetReport::class => \FlowCatalyst\Generated\Normalizer\BudgetReportNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkImportRequest::class => \FlowCatalyst\Generated\Normalizer\BulkImportRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkImportResponse::class => \FlowCatalyst\Generated\Normalizer\BulkImportResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkImportResult::class => \FlowCatalyst\Generated\Normalizer\BulkImportResultNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkImportUser::class => \FlowCatalyst\Generated\Normalizer\BulkImportUserNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkIssueFieldBadgesRequest::class => \FlowCatalyst\Generated\Normalizer\BulkIssueFieldBadgesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkIssueFieldBadgesResponse::class => \FlowCatalyst\Generated\Normalizer\BulkIssueFieldBadgesResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BulkIssuedFieldBadge::class => \FlowCatalyst\Generated\Normalizer\BulkIssuedFieldBadgeNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CheckEmailDomainResponse::class => \FlowCatalyst\Generated\Normalizer\CheckEmailDomainResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientAccessGrantListResponse::class => \FlowCatalyst\Generated\Normalizer\ClientAccessGrantListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientAccessGrantResponse::class => \FlowCatalyst\Generated\Normalizer\ClientAccessGrantResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientApplicationResponse::class => \FlowCatalyst\Generated\Normalizer\ClientApplicationResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientApplicationsResponse::class => \FlowCatalyst\Generated\Normalizer\ClientApplicationsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientAssociationRequest::class => \FlowCatalyst\Generated\Normalizer\ClientAssociationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientConfigListResponse::class => \FlowCatalyst\Generated\Normalizer\ClientConfigListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientConfigResponse::class => \FlowCatalyst\Generated\Normalizer\ClientConfigResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientListResponse::class => \FlowCatalyst\Generated\Normalizer\ClientListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ClientResponse::class => \FlowCatalyst\Generated\Normalizer\ClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CompleteInstanceRequest::class => \FlowCatalyst\Generated\Normalizer\CompleteInstanceRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConfigEntryDTO::class => \FlowCatalyst\Generated\Normalizer\ConfigEntryDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConfigListResponse::class => \FlowCatalyst\Generated\Normalizer\ConfigListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConfigResponse::class => \FlowCatalyst\Generated\Normalizer\ConfigResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConnectionListResponse::class => \FlowCatalyst\Generated\Normalizer\ConnectionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ConnectionResponse::class => \FlowCatalyst\Generated\Normalizer\ConnectionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ContextEntryDTO::class => \FlowCatalyst\Generated\Normalizer\ContextEntryDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\CorsOriginListResponse::class => \FlowCatalyst\Generated\Normalizer\CorsOriginListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateAnchorDomainRequest::class => \FlowCatalyst\Generated\Normalizer\CreateAnchorDomainRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateApplicationRequest::class => \FlowCatalyst\Generated\Normalizer\CreateApplicationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateAuthConfigRequest::class => \FlowCatalyst\Generated\Normalizer\CreateAuthConfigRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateClientRequest::class => \FlowCatalyst\Generated\Normalizer\CreateClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateConnectionRequest::class => \FlowCatalyst\Generated\Normalizer\CreateConnectionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateDispatchPoolRequest::class => \FlowCatalyst\Generated\Normalizer\CreateDispatchPoolRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateDomainRequest::class => \FlowCatalyst\Generated\Normalizer\CreateDomainRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEnrolmentCodeRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEnrolmentCodeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEventRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventResponse::class => \FlowCatalyst\Generated\Normalizer\CreateEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateEventTypeRequest::class => \FlowCatalyst\Generated\Normalizer\CreateEventTypeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateFieldSignInCodeRequest::class => \FlowCatalyst\Generated\Normalizer\CreateFieldSignInCodeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateFunctionRequest::class => \FlowCatalyst\Generated\Normalizer\CreateFunctionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateIdentityProviderRequest::class => \FlowCatalyst\Generated\Normalizer\CreateIdentityProviderRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateIdpRoleMappingRequest::class => \FlowCatalyst\Generated\Normalizer\CreateIdpRoleMappingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateIngestionRequest::class => \FlowCatalyst\Generated\Normalizer\CreateIngestionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateJobsRequest::class => \FlowCatalyst\Generated\Normalizer\CreateJobsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateJobsResponse::class => \FlowCatalyst\Generated\Normalizer\CreateJobsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateMappingRequest::class => \FlowCatalyst\Generated\Normalizer\CreateMappingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateOAuthClientRequest::class => \FlowCatalyst\Generated\Normalizer\CreateOAuthClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateOAuthClientResponse::class => \FlowCatalyst\Generated\Normalizer\CreateOAuthClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatePortalAppRequest::class => \FlowCatalyst\Generated\Normalizer\CreatePortalAppRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatePortalAppResponse::class => \FlowCatalyst\Generated\Normalizer\CreatePortalAppResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatePrincipalRequest::class => \FlowCatalyst\Generated\Normalizer\CreatePrincipalRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatePrincipalResponse::class => \FlowCatalyst\Generated\Normalizer\CreatePrincipalResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateProcessRequest::class => \FlowCatalyst\Generated\Normalizer\CreateProcessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateRoleRequest::class => \FlowCatalyst\Generated\Normalizer\CreateRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateScheduledJobRequest::class => \FlowCatalyst\Generated\Normalizer\CreateScheduledJobRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateServiceAccountRequest::class => \FlowCatalyst\Generated\Normalizer\CreateServiceAccountRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateServiceAccountResponse::class => \FlowCatalyst\Generated\Normalizer\CreateServiceAccountResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateSubscriptionRequest::class => \FlowCatalyst\Generated\Normalizer\CreateSubscriptionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreateUserRequest::class => \FlowCatalyst\Generated\Normalizer\CreateUserRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatedEvent::class => \FlowCatalyst\Generated\Normalizer\CreatedEventNormalizer::class,
        
        \FlowCatalyst\Generated\Model\CreatedResponse::class => \FlowCatalyst\Generated\Normalizer\CreatedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DeliveryPlan::class => \FlowCatalyst\Generated\Normalizer\DeliveryPlanNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Desired::class => \FlowCatalyst\Generated\Normalizer\DesiredNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DeveloperUserListResponse::class => \FlowCatalyst\Generated\Normalizer\DeveloperUserListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Dispatch::class => \FlowCatalyst\Generated\Normalizer\DispatchNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchJobFilterOptionsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchJobRead::class => \FlowCatalyst\Generated\Normalizer\DispatchJobReadNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchPoolListResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchPoolListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchPoolResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchPoolResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchRequest::class => \FlowCatalyst\Generated\Normalizer\DispatchRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DispatchResponse::class => \FlowCatalyst\Generated\Normalizer\DispatchResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DocListResponse::class => \FlowCatalyst\Generated\Normalizer\DocListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DocResponse::class => \FlowCatalyst\Generated\Normalizer\DocResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DocSummary::class => \FlowCatalyst\Generated\Normalizer\DocSummaryNormalizer::class,
        
        \FlowCatalyst\Generated\Model\DomainResponse::class => \FlowCatalyst\Generated\Normalizer\DomainResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EditPayloadRequest::class => \FlowCatalyst\Generated\Normalizer\EditPayloadRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EmitRequest::class => \FlowCatalyst\Generated\Normalizer\EmitRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EmitResponse::class => \FlowCatalyst\Generated\Normalizer\EmitResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EnableTraceRequest::class => \FlowCatalyst\Generated\Normalizer\EnableTraceRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EnsureFieldWorkerRequest::class => \FlowCatalyst\Generated\Normalizer\EnsureFieldWorkerRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ErrorModel::class => \FlowCatalyst\Generated\Normalizer\ErrorModelNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Event::class => \FlowCatalyst\Generated\Normalizer\EventNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventFilterOption::class => \FlowCatalyst\Generated\Normalizer\EventFilterOptionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventFilterOptionsResponse::class => \FlowCatalyst\Generated\Normalizer\EventFilterOptionsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventRead::class => \FlowCatalyst\Generated\Normalizer\EventReadNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventResponse::class => \FlowCatalyst\Generated\Normalizer\EventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeBindingDTO::class => \FlowCatalyst\Generated\Normalizer\EventTypeBindingDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeListResponse::class => \FlowCatalyst\Generated\Normalizer\EventTypeListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\EventTypeResponse::class => \FlowCatalyst\Generated\Normalizer\EventTypeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ExchangeWindow::class => \FlowCatalyst\Generated\Normalizer\ExchangeWindowNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Failure::class => \FlowCatalyst\Generated\Normalizer\FailureNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldAppRef::class => \FlowCatalyst\Generated\Normalizer\FieldAppRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldBadgeIssuedResponse::class => \FlowCatalyst\Generated\Normalizer\FieldBadgeIssuedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldBadgeListResponse::class => \FlowCatalyst\Generated\Normalizer\FieldBadgeListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldBadgeResponse::class => \FlowCatalyst\Generated\Normalizer\FieldBadgeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldDeviceListResponse::class => \FlowCatalyst\Generated\Normalizer\FieldDeviceListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldDeviceResponse::class => \FlowCatalyst\Generated\Normalizer\FieldDeviceResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldEnrolmentCodeResponse::class => \FlowCatalyst\Generated\Normalizer\FieldEnrolmentCodeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldError::class => \FlowCatalyst\Generated\Normalizer\FieldErrorNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldPolicyResponse::class => \FlowCatalyst\Generated\Normalizer\FieldPolicyResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldSignInCodeResponse::class => \FlowCatalyst\Generated\Normalizer\FieldSignInCodeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldTokenActor::class => \FlowCatalyst\Generated\Normalizer\FieldTokenActorNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldTokenIntrospectRequest::class => \FlowCatalyst\Generated\Normalizer\FieldTokenIntrospectRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldTokenIntrospectResponse::class => \FlowCatalyst\Generated\Normalizer\FieldTokenIntrospectResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerAppRef::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerAppRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerBadgeRef::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerBadgeRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerListResponse::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerResponse::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerSignInListResponse::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerSignInListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FieldWorkerSignInResponse::class => \FlowCatalyst\Generated\Normalizer\FieldWorkerSignInResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FireNowRequest::class => \FlowCatalyst\Generated\Normalizer\FireNowRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FireNowResponse::class => \FlowCatalyst\Generated\Normalizer\FireNowResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\_Function::class => \FlowCatalyst\Generated\Normalizer\FunctionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FunctionResponse::class => \FlowCatalyst\Generated\Normalizer\FunctionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FunctionTokenRequest::class => \FlowCatalyst\Generated\Normalizer\FunctionTokenRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\FunctionTokenResponse::class => \FlowCatalyst\Generated\Normalizer\FunctionTokenResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantAccessRequest::class => \FlowCatalyst\Generated\Normalizer\GrantAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantClientAccessRequest::class => \FlowCatalyst\Generated\Normalizer\GrantClientAccessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantFieldWorkerAppRequest::class => \FlowCatalyst\Generated\Normalizer\GrantFieldWorkerAppRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\GrantPermissionRequest::class => \FlowCatalyst\Generated\Normalizer\GrantPermissionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Heartbeat::class => \FlowCatalyst\Generated\Normalizer\HeartbeatNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdentityProviderListResponse::class => \FlowCatalyst\Generated\Normalizer\IdentityProviderListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdentityProviderResponse::class => \FlowCatalyst\Generated\Normalizer\IdentityProviderResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdpRoleMappingListResponse::class => \FlowCatalyst\Generated\Normalizer\IdpRoleMappingListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IdpRoleMappingResponse::class => \FlowCatalyst\Generated\Normalizer\IdpRoleMappingResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionConfigDeletedResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionConfigDeletedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionConfigResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionConfigResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionConfigSecret::class => \FlowCatalyst\Generated\Normalizer\IngestionConfigSecretNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionConfigStatus::class => \FlowCatalyst\Generated\Normalizer\IngestionConfigStatusNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionConfigWritten::class => \FlowCatalyst\Generated\Normalizer\IngestionConfigWrittenNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionDefinition::class => \FlowCatalyst\Generated\Normalizer\IngestionDefinitionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionEnableTraceRequest::class => \FlowCatalyst\Generated\Normalizer\IngestionEnableTraceRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionFailureListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionFailureListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionFailureResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionFailureResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionHistoryEntry::class => \FlowCatalyst\Generated\Normalizer\IngestionHistoryEntryNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionHistoryResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionHistoryResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSaveOutcome::class => \FlowCatalyst\Generated\Normalizer\IngestionSaveOutcomeNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSecretFetchResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSecretFetchResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSecretListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSecretListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSecretResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSecretResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetApplyListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetApplyListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetApplyResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetApplyResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetArchiveResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetArchiveResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetItem::class => \FlowCatalyst\Generated\Normalizer\IngestionSetItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetPlanResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetPlanResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetReleaseResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetReleaseResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSetResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSetResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSourceKindListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSourceKindListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionSourceKindResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionSourceKindResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTestResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionTestResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceAck::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceAckNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceBatch::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceBatchNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceLogResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceLogResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceRecord::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceRecordNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceRecordResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceRecordResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionTraceStateResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionTraceStateResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionVersionListResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionVersionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IngestionVersionResponse::class => \FlowCatalyst\Generated\Normalizer\IngestionVersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\InviteApplicationUserRequest::class => \FlowCatalyst\Generated\Normalizer\InviteApplicationUserRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\InviteApplicationUserResponse::class => \FlowCatalyst\Generated\Normalizer\InviteApplicationUserResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\IssueFieldBadgeRequest::class => \FlowCatalyst\Generated\Normalizer\IssueFieldBadgeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\JobRecord::class => \FlowCatalyst\Generated\Normalizer\JobRecordNormalizer::class,
        
        \FlowCatalyst\Generated\Model\JobResult::class => \FlowCatalyst\Generated\Normalizer\JobResultNormalizer::class,
        
        \FlowCatalyst\Generated\Model\KV::class => \FlowCatalyst\Generated\Normalizer\KVNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Limits::class => \FlowCatalyst\Generated\Normalizer\LimitsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\LimitsDTO::class => \FlowCatalyst\Generated\Normalizer\LimitsDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\LinkedOAuthClient::class => \FlowCatalyst\Generated\Normalizer\LinkedOAuthClientNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ListOutputBody::class => \FlowCatalyst\Generated\Normalizer\ListOutputBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\LoginAttemptListResponse::class => \FlowCatalyst\Generated\Normalizer\LoginAttemptListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\LoginAttemptResponse::class => \FlowCatalyst\Generated\Normalizer\LoginAttemptResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MappingListResponse::class => \FlowCatalyst\Generated\Normalizer\MappingListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MappingResponse::class => \FlowCatalyst\Generated\Normalizer\MappingResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MatchEmailsRequest::class => \FlowCatalyst\Generated\Normalizer\MatchEmailsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MatchEmailsResponse::class => \FlowCatalyst\Generated\Normalizer\MatchEmailsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MetadataDTO::class => \FlowCatalyst\Generated\Normalizer\MetadataDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\MoveProviderRequest::class => \FlowCatalyst\Generated\Normalizer\MoveProviderRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\MoveProviderResponse::class => \FlowCatalyst\Generated\Normalizer\MoveProviderResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\NoteResponse::class => \FlowCatalyst\Generated\Normalizer\NoteResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OAuthClientApplicationRef::class => \FlowCatalyst\Generated\Normalizer\OAuthClientApplicationRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OAuthClientListResponse::class => \FlowCatalyst\Generated\Normalizer\OAuthClientListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OAuthClientResponse::class => \FlowCatalyst\Generated\Normalizer\OAuthClientResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OffsetPageFunctionResponse::class => \FlowCatalyst\Generated\Normalizer\OffsetPageFunctionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OffsetPageScheduledJobInstanceResponse::class => \FlowCatalyst\Generated\Normalizer\OffsetPageScheduledJobInstanceResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\OffsetPageScheduledJobResponse::class => \FlowCatalyst\Generated\Normalizer\OffsetPageScheduledJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionListResponse::class => \FlowCatalyst\Generated\Normalizer\PermissionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PermissionResponse::class => \FlowCatalyst\Generated\Normalizer\PermissionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PollRequest::class => \FlowCatalyst\Generated\Normalizer\PollRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PollResponse::class => \FlowCatalyst\Generated\Normalizer\PollResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PoolConfig::class => \FlowCatalyst\Generated\Normalizer\PoolConfigNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalAppListResponse::class => \FlowCatalyst\Generated\Normalizer\PortalAppListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalAppResponse::class => \FlowCatalyst\Generated\Normalizer\PortalAppResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserAppGrantBody::class => \FlowCatalyst\Generated\Normalizer\PortalUserAppGrantBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserAppRef::class => \FlowCatalyst\Generated\Normalizer\PortalUserAppRefNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserClientBody::class => \FlowCatalyst\Generated\Normalizer\PortalUserClientBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserListItem::class => \FlowCatalyst\Generated\Normalizer\PortalUserListItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserListResponse::class => \FlowCatalyst\Generated\Normalizer\PortalUserListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserRequest::class => \FlowCatalyst\Generated\Normalizer\PortalUserRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PortalUserResponse::class => \FlowCatalyst\Generated\Normalizer\PortalUserResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalAvailableApplication::class => \FlowCatalyst\Generated\Normalizer\PrincipalAvailableApplicationNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalAvailableApplicationsResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalAvailableApplicationsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalListResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalRoleAssignmentDTO::class => \FlowCatalyst\Generated\Normalizer\PrincipalRoleAssignmentDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalRoleListResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalRoleListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrincipalVersionResponse::class => \FlowCatalyst\Generated\Normalizer\PrincipalVersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ProcessListResponse::class => \FlowCatalyst\Generated\Normalizer\ProcessListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ProcessResponse::class => \FlowCatalyst\Generated\Normalizer\ProcessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ProvisionLoginClientRequest::class => \FlowCatalyst\Generated\Normalizer\ProvisionLoginClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PruneVersionsRequest::class => \FlowCatalyst\Generated\Normalizer\PruneVersionsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PruneVersionsResponse::class => \FlowCatalyst\Generated\Normalizer\PruneVersionsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PrunedVersionResponse::class => \FlowCatalyst\Generated\Normalizer\PrunedVersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PublicAllowedResponse::class => \FlowCatalyst\Generated\Normalizer\PublicAllowedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PublishVersionRequest::class => \FlowCatalyst\Generated\Normalizer\PublishVersionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PutAliasRequest::class => \FlowCatalyst\Generated\Normalizer\PutAliasRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PutAliasResponse::class => \FlowCatalyst\Generated\Normalizer\PutAliasResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PutFieldPolicyRequest::class => \FlowCatalyst\Generated\Normalizer\PutFieldPolicyRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PutRouteRequest::class => \FlowCatalyst\Generated\Normalizer\PutRouteRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\PutSettingRequest::class => \FlowCatalyst\Generated\Normalizer\PutSettingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\QueueConfig::class => \FlowCatalyst\Generated\Normalizer\QueueConfigNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RawDispatchJobResponse::class => \FlowCatalyst\Generated\Normalizer\RawDispatchJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RawEventResponse::class => \FlowCatalyst\Generated\Normalizer\RawEventResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegenerateAuthTokenResponse::class => \FlowCatalyst\Generated\Normalizer\RegenerateAuthTokenResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegenerateSigningSecretResponse::class => \FlowCatalyst\Generated\Normalizer\RegenerateSigningSecretResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegisterBeginRequest::class => \FlowCatalyst\Generated\Normalizer\RegisterBeginRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegisterBeginResponse::class => \FlowCatalyst\Generated\Normalizer\RegisterBeginResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegisterCompleteRequest::class => \FlowCatalyst\Generated\Normalizer\RegisterCompleteRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RegisterCompleteResponse::class => \FlowCatalyst\Generated\Normalizer\RegisterCompleteResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ReleaseRequest::class => \FlowCatalyst\Generated\Normalizer\ReleaseRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ReleaseResponse::class => \FlowCatalyst\Generated\Normalizer\ReleaseResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ReportRequest::class => \FlowCatalyst\Generated\Normalizer\ReportRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequestDTO::class => \FlowCatalyst\Generated\Normalizer\RequestDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequestSummary::class => \FlowCatalyst\Generated\Normalizer\RequestSummaryNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueByFilterPreviewRequest::class => \FlowCatalyst\Generated\Normalizer\RequeueByFilterPreviewRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueByFilterPreviewResponse::class => \FlowCatalyst\Generated\Normalizer\RequeueByFilterPreviewResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueByFilterRequest::class => \FlowCatalyst\Generated\Normalizer\RequeueByFilterRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueByFilterResponse::class => \FlowCatalyst\Generated\Normalizer\RequeueByFilterResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueRequest::class => \FlowCatalyst\Generated\Normalizer\RequeueRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RequeueResponse::class => \FlowCatalyst\Generated\Normalizer\RequeueResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResendInvitationRequest::class => \FlowCatalyst\Generated\Normalizer\ResendInvitationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResendInvitationResponse::class => \FlowCatalyst\Generated\Normalizer\ResendInvitationResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResetFieldWorkerPinRequest::class => \FlowCatalyst\Generated\Normalizer\ResetFieldWorkerPinRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResetFieldWorkerPinResponse::class => \FlowCatalyst\Generated\Normalizer\ResetFieldWorkerPinResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ResetPasswordRequest::class => \FlowCatalyst\Generated\Normalizer\ResetPasswordRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleAssignmentDTO::class => \FlowCatalyst\Generated\Normalizer\RoleAssignmentDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleListResponse::class => \FlowCatalyst\Generated\Normalizer\RoleListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RolePermissionListResponse::class => \FlowCatalyst\Generated\Normalizer\RolePermissionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RoleResponse::class => \FlowCatalyst\Generated\Normalizer\RoleResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RolesAssignedResponse::class => \FlowCatalyst\Generated\Normalizer\RolesAssignedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RotateOAuthClientSecretRequest::class => \FlowCatalyst\Generated\Normalizer\RotateOAuthClientSecretRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RotateOAuthClientSecretResponse::class => \FlowCatalyst\Generated\Normalizer\RotateOAuthClientSecretResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Route::class => \FlowCatalyst\Generated\Normalizer\RouteNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RouteResponse::class => \FlowCatalyst\Generated\Normalizer\RouteResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RouteState::class => \FlowCatalyst\Generated\Normalizer\RouteStateNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RouterConfig::class => \FlowCatalyst\Generated\Normalizer\RouterConfigNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RunnerCapabilities::class => \FlowCatalyst\Generated\Normalizer\RunnerCapabilitiesNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RuntimeIngestion::class => \FlowCatalyst\Generated\Normalizer\RuntimeIngestionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RuntimeKind::class => \FlowCatalyst\Generated\Normalizer\RuntimeKindNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RuntimeTest::class => \FlowCatalyst\Generated\Normalizer\RuntimeTestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\RuntimeTrace::class => \FlowCatalyst\Generated\Normalizer\RuntimeTraceNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SaveIngestionVersionRequest::class => \FlowCatalyst\Generated\Normalizer\SaveIngestionVersionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ScheduledJobInstanceLogResponse::class => \FlowCatalyst\Generated\Normalizer\ScheduledJobInstanceLogResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ScheduledJobInstanceResponse::class => \FlowCatalyst\Generated\Normalizer\ScheduledJobInstanceResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ScheduledJobResponse::class => \FlowCatalyst\Generated\Normalizer\ScheduledJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SearchClientRequest::class => \FlowCatalyst\Generated\Normalizer\SearchClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SecretResponse::class => \FlowCatalyst\Generated\Normalizer\SecretResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SecretValue::class => \FlowCatalyst\Generated\Normalizer\SecretValueNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SendPasswordResetInputBody::class => \FlowCatalyst\Generated\Normalizer\SendPasswordResetInputBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountListResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountOAuthSecrets::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountOAuthSecretsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountRoleListResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountRoleListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountRolesAssignedResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountRolesAssignedResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountTokenResponse::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountTokenResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ServiceAccountWebhookSecrets::class => \FlowCatalyst\Generated\Normalizer\ServiceAccountWebhookSecretsNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetApplicationAccessResponse::class => \FlowCatalyst\Generated\Normalizer\SetApplicationAccessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetChange::class => \FlowCatalyst\Generated\Normalizer\SetChangeNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetDeveloperCredentialResponse::class => \FlowCatalyst\Generated\Normalizer\SetDeveloperCredentialResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetFile::class => \FlowCatalyst\Generated\Normalizer\SetFileNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetIngestionConfigRequest::class => \FlowCatalyst\Generated\Normalizer\SetIngestionConfigRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SetPropertyRequest::class => \FlowCatalyst\Generated\Normalizer\SetPropertyRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SettingResponse::class => \FlowCatalyst\Generated\Normalizer\SettingResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SkippedJobResponse::class => \FlowCatalyst\Generated\Normalizer\SkippedJobResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SpecVersionResponse::class => \FlowCatalyst\Generated\Normalizer\SpecVersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StateValue::class => \FlowCatalyst\Generated\Normalizer\StateValueNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusChangeRequest::class => \FlowCatalyst\Generated\Normalizer\StatusChangeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\StatusChangeResponse::class => \FlowCatalyst\Generated\Normalizer\StatusChangeResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionListResponse::class => \FlowCatalyst\Generated\Normalizer\SubscriptionListResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SubscriptionResponse::class => \FlowCatalyst\Generated\Normalizer\SubscriptionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SuccessResponse::class => \FlowCatalyst\Generated\Normalizer\SuccessResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SuspendClientRequest::class => \FlowCatalyst\Generated\Normalizer\SuspendClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncConnectionInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncConnectionInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncConnectionsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncConnectionsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncDispatchPoolInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncDispatchPoolInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncDispatchPoolsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncDispatchPoolsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncDocInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncDocInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncDocsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncDocsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncEventTypeInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncEventTypeInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncEventTypesRequest::class => \FlowCatalyst\Generated\Normalizer\SyncEventTypesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncOpenApiSpecResponse::class => \FlowCatalyst\Generated\Normalizer\SyncOpenApiSpecResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncOpenapiRequest::class => \FlowCatalyst\Generated\Normalizer\SyncOpenapiRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncPrincipalInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncPrincipalInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncPrincipalsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncPrincipalsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncProcessInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncProcessInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncProcessesByBodyRequest::class => \FlowCatalyst\Generated\Normalizer\SyncProcessesByBodyRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncProcessesRequest::class => \FlowCatalyst\Generated\Normalizer\SyncProcessesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncResultResponse::class => \FlowCatalyst\Generated\Normalizer\SyncResultResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncRoleInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncRoleInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncRolesRequest::class => \FlowCatalyst\Generated\Normalizer\SyncRolesRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncScheduledJobInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncScheduledJobInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncScheduledJobsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncScheduledJobsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncScheduledJobsResultResponse::class => \FlowCatalyst\Generated\Normalizer\SyncScheduledJobsResultResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncSubscriptionEventTypeRequest::class => \FlowCatalyst\Generated\Normalizer\SyncSubscriptionEventTypeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncSubscriptionInputRequest::class => \FlowCatalyst\Generated\Normalizer\SyncSubscriptionInputRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncSubscriptionsRequest::class => \FlowCatalyst\Generated\Normalizer\SyncSubscriptionsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncUserInput::class => \FlowCatalyst\Generated\Normalizer\SyncUserInputNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncUsersRequest::class => \FlowCatalyst\Generated\Normalizer\SyncUsersRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\SyncUsersResponse::class => \FlowCatalyst\Generated\Normalizer\SyncUsersResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Target::class => \FlowCatalyst\Generated\Normalizer\TargetNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TestAnswer::class => \FlowCatalyst\Generated\Normalizer\TestAnswerNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TestIngestionRequest::class => \FlowCatalyst\Generated\Normalizer\TestIngestionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TokenAuth::class => \FlowCatalyst\Generated\Normalizer\TokenAuthNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceAck::class => \FlowCatalyst\Generated\Normalizer\TraceAckNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceBatch::class => \FlowCatalyst\Generated\Normalizer\TraceBatchNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceContextResponse::class => \FlowCatalyst\Generated\Normalizer\TraceContextResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceContextsResponse::class => \FlowCatalyst\Generated\Normalizer\TraceContextsResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceLogResponse::class => \FlowCatalyst\Generated\Normalizer\TraceLogResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceRecord::class => \FlowCatalyst\Generated\Normalizer\TraceRecordNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceRecordResponse::class => \FlowCatalyst\Generated\Normalizer\TraceRecordResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\TraceState::class => \FlowCatalyst\Generated\Normalizer\TraceStateNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateAnchorDomainRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateAnchorDomainRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateApplicationRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateApplicationRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateAuthConfigRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateAuthConfigRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateClientApplicationsRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateClientRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateConnectionRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateConnectionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateDispatchPoolRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateDispatchPoolRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateEventTypeRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateEventTypeRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateFieldWorkerRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateFieldWorkerRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateFunctionRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateFunctionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateIdentityProviderRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateIngestionRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateIngestionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateMappingRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateMappingRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateOAuthClientRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateOAuthClientRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdatePortalAppRequest::class => \FlowCatalyst\Generated\Normalizer\UpdatePortalAppRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdatePrincipalRequest::class => \FlowCatalyst\Generated\Normalizer\UpdatePrincipalRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateProcessRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateProcessRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateRoleRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateRoleRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateScheduledJobRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateScheduledJobRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateServiceAccountRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateServiceAccountRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\UpdateSubscriptionRequest::class => \FlowCatalyst\Generated\Normalizer\UpdateSubscriptionRequestNormalizer::class,
        
        \FlowCatalyst\Generated\Model\Version::class => \FlowCatalyst\Generated\Normalizer\VersionNormalizer::class,
        
        \FlowCatalyst\Generated\Model\VersionReport::class => \FlowCatalyst\Generated\Normalizer\VersionReportNormalizer::class,
        
        \FlowCatalyst\Generated\Model\VersionResponse::class => \FlowCatalyst\Generated\Normalizer\VersionResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WebauthnAuthenticateCompleteResponse::class => \FlowCatalyst\Generated\Normalizer\WebauthnAuthenticateCompleteResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WebauthnCredentialSummary::class => \FlowCatalyst\Generated\Normalizer\WebauthnCredentialSummaryNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WebhookCredentialsDTO::class => \FlowCatalyst\Generated\Normalizer\WebhookCredentialsDTONormalizer::class,
        
        \FlowCatalyst\Generated\Model\WindowResponse::class => \FlowCatalyst\Generated\Normalizer\WindowResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WiringResponse::class => \FlowCatalyst\Generated\Normalizer\WiringResponseNormalizer::class,
        
        \FlowCatalyst\Generated\Model\WriteInstanceLogRequest::class => \FlowCatalyst\Generated\Normalizer\WriteInstanceLogRequestNormalizer::class,
        
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
        return array_combine(array_keys($this->normalizers), array_fill(0, count($this->normalizers), false));
    }
}