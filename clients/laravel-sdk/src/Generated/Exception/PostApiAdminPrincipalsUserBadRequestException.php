<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsUserBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400
     */
    private $apiAdminPrincipalsUsersPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400 $apiAdminPrincipalsUsersPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsUsersPostResponse400 = $apiAdminPrincipalsUsersPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsUsersPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse400
    {
        return $this->apiAdminPrincipalsUsersPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}