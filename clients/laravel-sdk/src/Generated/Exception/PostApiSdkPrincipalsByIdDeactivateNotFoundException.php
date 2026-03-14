<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse404
     */
    private $apiSdkPrincipalsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse404 $apiSdkPrincipalsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdDeactivatePostResponse404 = $apiSdkPrincipalsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse404
    {
        return $this->apiSdkPrincipalsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}