<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdArchiveNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse404
     */
    private $apiAdminEventTypesIdArchivePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse404 $apiAdminEventTypesIdArchivePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdArchivePostResponse404 = $apiAdminEventTypesIdArchivePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdArchivePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse404
    {
        return $this->apiAdminEventTypesIdArchivePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}