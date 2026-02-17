<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminOauthClientsByClientIdByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse404
     */
    private $apiAdminOauthClientsByClientIdClientIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse404 $apiAdminOauthClientsByClientIdClientIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsByClientIdClientIdGetResponse404 = $apiAdminOauthClientsByClientIdClientIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsByClientIdClientIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse404
    {
        return $this->apiAdminOauthClientsByClientIdClientIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}