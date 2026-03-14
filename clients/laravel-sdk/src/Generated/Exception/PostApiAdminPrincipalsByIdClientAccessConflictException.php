<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdClientAccessConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409
     */
    private $apiAdminPrincipalsIdClientAccessPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409 $apiAdminPrincipalsIdClientAccessPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdClientAccessPostResponse409 = $apiAdminPrincipalsIdClientAccessPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdClientAccessPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse409
    {
        return $this->apiAdminPrincipalsIdClientAccessPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}