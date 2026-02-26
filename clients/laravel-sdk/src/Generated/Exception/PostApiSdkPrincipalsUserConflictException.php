<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsUserConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409
     */
    private $apiSdkPrincipalsUserPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409 $apiSdkPrincipalsUserPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsUserPostResponse409 = $apiSdkPrincipalsUserPostResponse409;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsUserPostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse409
    {
        return $this->apiSdkPrincipalsUserPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}