<?php

namespace FlowCatalyst\Generated\Exception;

class PatchApiAdminEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse404
     */
    private $apiAdminEventTypesIdPatchResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse404 $apiAdminEventTypesIdPatchResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdPatchResponse404 = $apiAdminEventTypesIdPatchResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdPatchResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse404
    {
        return $this->apiAdminEventTypesIdPatchResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}