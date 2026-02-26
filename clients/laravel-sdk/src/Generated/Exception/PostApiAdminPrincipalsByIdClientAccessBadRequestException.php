<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdClientAccessBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400
     */
    private $apiAdminPrincipalsIdClientAccessPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400 $apiAdminPrincipalsIdClientAccessPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdClientAccessPostResponse400 = $apiAdminPrincipalsIdClientAccessPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdClientAccessPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse400
    {
        return $this->apiAdminPrincipalsIdClientAccessPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}