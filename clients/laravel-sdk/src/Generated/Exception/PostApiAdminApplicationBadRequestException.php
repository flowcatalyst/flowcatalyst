<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse400
     */
    private $apiAdminApplicationsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse400 $apiAdminApplicationsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsPostResponse400 = $apiAdminApplicationsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminApplicationsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse400
    {
        return $this->apiAdminApplicationsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}