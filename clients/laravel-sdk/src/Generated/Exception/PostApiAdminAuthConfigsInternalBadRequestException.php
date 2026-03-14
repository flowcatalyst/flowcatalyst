<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAuthConfigsInternalBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400
     */
    private $apiAdminAuthConfigsInternalPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400 $apiAdminAuthConfigsInternalPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsInternalPostResponse400 = $apiAdminAuthConfigsInternalPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsInternalPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse400
    {
        return $this->apiAdminAuthConfigsInternalPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}