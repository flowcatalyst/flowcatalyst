<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsUserConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409
     */
    private $apiAdminPrincipalsUsersPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409 $apiAdminPrincipalsUsersPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsUsersPostResponse409 = $apiAdminPrincipalsUsersPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsUsersPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse409
    {
        return $this->apiAdminPrincipalsUsersPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}