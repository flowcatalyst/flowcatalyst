<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsUserBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400
     */
    private $apiSdkPrincipalsUserPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400 $apiSdkPrincipalsUserPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsUserPostResponse400 = $apiSdkPrincipalsUserPostResponse400;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsUserPostResponse400(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse400
    {
        return $this->apiSdkPrincipalsUserPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}