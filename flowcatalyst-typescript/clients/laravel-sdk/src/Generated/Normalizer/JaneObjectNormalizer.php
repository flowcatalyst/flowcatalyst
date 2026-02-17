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
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsGetResponse200PrincipalsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsGetResponse200PrincipalsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsUsersPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsUsersPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsUsersPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsUsersPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdResetPasswordPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdResetPasswordPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdResetPasswordPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdResetPasswordPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPutResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsCheckEmailDomainGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsCheckEmailDomainGetResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessPutResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdApplicationAccessPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200ClientsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsGetResponse200ClientsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200ClientsItemNotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsGetResponse200ClientsItemNotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse201NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsPostResponse201NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsSearchGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200ClientsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsSearchGetResponse200ClientsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200ClientsItemNotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsSearchGetResponse200ClientsItemNotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdGetResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsByIdentifierIdentifierGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsByIdentifierIdentifierGetResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsByIdentifierIdentifierGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdActivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdSuspendPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdDeactivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdNotesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdNotesPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse200NotesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdNotesPostResponse200NotesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdNotesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdNotesPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsAppIdEnablePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsGetResponse200AnchorDomainsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsGetResponse200AnchorDomainsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAnchorDomainsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsByCodeCodeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsByCodeCodeGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdActivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdDeactivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse200ConfigsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsGetResponse200ConfigsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsClientIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdClientsClientIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdRolesGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdProvisionServiceAccountPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNameDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNameDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNameGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNameGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNamePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNamePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNamePutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNamePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesNamePutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesBySourceSourceGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesBySourceSourceGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesBySourceSourceGetResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesByApplicationApplicationIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesByApplicationApplicationIdGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPermissionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsGetResponse200PermissionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPermissionsGetResponse200PermissionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPermissionsPermissionGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminRolesPermissionsPermissionGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsGetResponse200ConfigsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsGetResponse200ConfigsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsByDomainDomainGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsByDomainDomainGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsInternalPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsInternalPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsInternalPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsInternalPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsOidcPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsOidcPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsOidcPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsOidcPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdOidcPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdOidcPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdOidcPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdOidcPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdOidcPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdConfigTypePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdConfigTypePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdConfigTypePutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdConfigTypePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdConfigTypePutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdAdditionalClientsPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdAdditionalClientsPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGrantedClientsPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGrantedClientsPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGrantedClientsPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuthConfigsIdGrantedClientsPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200ClientsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsGetResponse200ClientsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200ClientsItemApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsGetResponse200ClientsItemApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse201ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsPostResponse201ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsByClientIdClientIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsByClientIdClientIdGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsByClientIdClientIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRegenerateSecretPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRegenerateSecretPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRegenerateSecretPostResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRegenerateSecretPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRegenerateSecretPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRotateSecretPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRotateSecretPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRotateSecretPostResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRotateSecretPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminOauthClientsIdRotateSecretPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsGetResponse200AuditLogsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsGetResponse200AuditLogsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200AuditLogsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200AuditLogsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityTypesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsEntityTypesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminAuditLogsOperationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminAuditLogsOperationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesGetResponse200EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200EventTypesItemSpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesGetResponse200EventTypesItemSpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdGetResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdPatchBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdPatchResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdPatchResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdPatchResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdPatchResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdArchivePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdArchivePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdArchivePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesSyncPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBodyEventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesSyncPostBodyEventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesSyncPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesSyncPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesFiltersApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersSubdomainsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesFiltersSubdomainsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersAggregatesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventTypesFiltersAggregatesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsGetResponse200PoolsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsGetResponse200PoolsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdSuspendPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdSuspendPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsSyncPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBodyPoolsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsSyncPostBodyPoolsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsSyncPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchPoolsSyncPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsGetResponse200SubscriptionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItemEventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsGetResponse200SubscriptionsItemEventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItemCustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsGetResponse200SubscriptionsItemCustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBodyEventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostBodyEventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBodyCustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostBodyCustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostResponse201EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201CustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostResponse201CustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdGetResponse200EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200CustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdGetResponse200CustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBodyEventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutBodyEventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBodyCustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutBodyCustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutResponse200EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200CustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutResponse200CustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPausePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPausePostResponse200EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200CustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPausePostResponse200CustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdPausePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdResumePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200EventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdResumePostResponse200EventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200CustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdResumePostResponse200CustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsIdResumePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemEventTypesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemEventTypesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemCustomConfigItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemCustomConfigItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminSubscriptionsSyncPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEventsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsIdAttemptsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminDispatchJobsIdAttemptsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersGetResponse200IdentityProvidersItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersGetResponse200IdentityProvidersItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminIdentityProvidersIdPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsGetResponse200MappingsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsIdPutResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsLookupDomainGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminEmailDomainMappingsLookupDomainGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsGetResponse200ServiceAccountsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsGetResponse200ServiceAccountsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201ServiceAccount::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse201ServiceAccountNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Oauth::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse201OauthNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Webhook::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse201WebhookNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsCodeCodeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsCodeCodeGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdAuthTokenPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdAuthTokenPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdAuthTokenPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdAuthTokenPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateTokenPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateSecretPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesPutResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminServiceAccountsIdRolesPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsGetResponse200CorsOriginsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsGetResponse200CorsOriginsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsAllowedGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsAllowedGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminPlatformCorsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeGetResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionGetResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyGetResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyPutResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAppCodeSectionPropertyPutResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodePostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodePostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeRoleCodePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeRoleCodePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiAdminConfigAccessAppCodeRoleCodePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200ItemsItemSpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesGetResponse200ItemsItemSpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesPostBody::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdGetResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdPatchBody::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdPatchBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdPatchResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdPatchResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse400::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdPatchResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdPatchResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdArchivePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdArchivePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdArchivePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdArchivePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostBody::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse201::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse400::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionFinalisePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionFinalisePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionFinalisePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionDeprecatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionDeprecatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesIdSchemasVersionDeprecatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesFiltersApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesFiltersApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesFiltersSubdomainsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesFiltersSubdomainsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventTypesFiltersAggregatesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventTypesFiltersAggregatesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffRolesGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPostBody::class => \FlowCatalyst\Generated\Normalizer\BffRolesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\BffRolesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\BffRolesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\BffRolesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesFiltersApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesFiltersApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesFiltersApplicationsGetResponse200OptionsItem::class => \FlowCatalyst\Generated\Normalizer\BffRolesFiltersApplicationsGetResponse200OptionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPermissionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesPermissionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPermissionsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffRolesPermissionsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesPermissionsPermissionGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffRolesPermissionsPermissionGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNameDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNameDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNameGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNameGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNamePutBody::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNamePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse200::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNamePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse400::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNamePutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse404::class => \FlowCatalyst\Generated\Normalizer\BffRolesRoleNamePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffEventsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffEventsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffEventsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffEventsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsIdAttemptsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsIdAttemptsGetResponse200AttemptsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffDispatchJobsIdAttemptsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200ItemsItemSpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesGetResponse200ItemsItemSpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdGetResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchBody::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdPatchBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdPatchResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdPatchResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdPatchResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdPatchResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdArchivePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdArchivePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdArchivePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdArchivePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostResponse201SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesFiltersApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersSubdomainsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesFiltersSubdomainsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersAggregatesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventTypesFiltersAggregatesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesFiltersApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesFiltersApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesFiltersApplicationsGetResponse200OptionsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesFiltersApplicationsGetResponse200OptionsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPermissionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPermissionsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPermissionsPermissionGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesPermissionsPermissionGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNameDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNameGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNameGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutBody::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNamePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNamePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNamePutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffRolesRoleNamePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffEventsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsFilterOptionsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsFilterOptionsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsIdAttemptsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsIdAttemptsGetResponse200AttemptsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffDispatchJobsIdAttemptsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsGetResponse200ClientsItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsGetResponse200ClientsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdActivatePostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdActivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdSuspendPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdSuspendPostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdSuspendPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdSuspendPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdDeactivatePostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkClientsIdDeactivatePostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNameDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNameGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNameGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNamePutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNamePutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNamePutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkRolesRoleNamePutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsGetResponse200PrincipalsItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsGetResponse200PrincipalsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsUserPostBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsUserPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsUserPostResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsUserPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdActivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdActivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdDeactivatePostResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdDeactivatePostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesGetResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesPutBodyNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesPutResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200RolesItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesPutResponse200RolesItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesPutResponse400Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdRolesPutResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse200GrantsItem::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsGetResponse200GrantsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse201::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsClientIdPostResponse201Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsClientIdPostResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409::class => \FlowCatalyst\Generated\Normalizer\ApiSdkPrincipalsIdClientsClientIdPostResponse409Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse200ClientsItem::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsGetResponse200ClientsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse401::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsGetResponse401Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdGetResponse401Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdGetResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdApplicationsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200ApplicationsItem::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdApplicationsGetResponse200ApplicationsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdApplicationsGetResponse401Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdApplicationsGetResponse403Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiMeClientsClientIdApplicationsGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiPublicLoginThemeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiPublicLoginThemeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiPublicPlatformGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiPublicPlatformGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiPublicPlatformGetResponse200Features::class => \FlowCatalyst\Generated\Normalizer\ApiPublicPlatformGetResponse200FeaturesNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiConfigLoginThemeGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiConfigPlatformGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiConfigPlatformGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiConfigPlatformGetResponse200Features::class => \FlowCatalyst\Generated\Normalizer\ApiConfigPlatformGetResponse200FeaturesNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugEventsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDebugEventsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugEventsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffDebugEventsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDebugEventsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffDebugEventsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugDispatchJobsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDebugDispatchJobsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugDispatchJobsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\BffDebugDispatchJobsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\BffDebugDispatchJobsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\BffDebugDispatchJobsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugEventsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugEventsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugEventsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugEventsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugEventsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugEventsIdGetResponse404Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugDispatchJobsGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsGetResponse200ItemsItem::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugDispatchJobsGetResponse200ItemsItemNormalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse200::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugDispatchJobsIdGetResponse200Normalizer::class,
        
        \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse404::class => \FlowCatalyst\Generated\Normalizer\ApiBffDebugDispatchJobsIdGetResponse404Normalizer::class,
        
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
            
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsGetResponse200PrincipalsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200ClientsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200ClientsItemNotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse201NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200ClientsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200ClientsItemNotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse200NotesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsGetResponse200AnchorDomainsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse200ConfigsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsClientIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNameDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsGetResponse200PermissionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsGetResponse200ConfigsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200ClientsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200ClientsItemApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse201ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsGetResponse200AuditLogsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200AuditLogsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityTypesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminAuditLogsOperationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200EventTypesItemSpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBodyEventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersSubdomainsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersAggregatesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsGetResponse200PoolsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBodyPoolsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItemEventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200SubscriptionsItemCustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBodyEventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBodyCustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201CustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200CustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBodyEventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBodyCustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200CustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200CustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200EventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200CustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemEventTypesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBodySubscriptionsItemCustomConfigItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersGetResponse200IdentityProvidersItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200MappingsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsGetResponse200ServiceAccountsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201ServiceAccount::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Oauth::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201Webhook::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsGetResponse200CorsOriginsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsAllowedGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesGetResponse200ItemsItemSpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesPostBody::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdPatchBody::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse400::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostBody::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesFiltersApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesFiltersSubdomainsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventTypesFiltersAggregatesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPostBody::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\BffRolesFiltersApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesFiltersApplicationsGetResponse200OptionsItem::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPermissionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPermissionsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNameDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNamePutBody::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse400::class => false,
            \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffEventsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffEventsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => false,
            \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200ItemsItemSpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchBody::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200SpecVersionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersSubdomainsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersAggregatesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesFiltersApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesFiltersApplicationsGetResponse200OptionsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsFilterOptionsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse200AttemptsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsGetResponse200ClientsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsGetResponse200PrincipalsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200RolesItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse200GrantsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse201::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse200ClientsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse401::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200ApplicationsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403::class => false,
            \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiPublicLoginThemeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiPublicPlatformGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiPublicPlatformGetResponse200Features::class => false,
            \FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiConfigPlatformGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiConfigPlatformGetResponse200Features::class => false,
            \FlowCatalyst\Generated\Model\BffDebugEventsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDebugEventsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\BffDebugDispatchJobsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDebugDispatchJobsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugEventsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugEventsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse404::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsGetResponse200ItemsItem::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse200::class => false,
            \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse404::class => false,
            \Jane\Component\JsonSchemaRuntime\Reference::class => false,
        ];
    }
}