<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesSyncBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse400
     */
    private $apiAdminEventTypesSyncPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse400 $apiAdminEventTypesSyncPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesSyncPostResponse400 = $apiAdminEventTypesSyncPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminEventTypesSyncPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse400
    {
        return $this->apiAdminEventTypesSyncPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}