<?php

namespace FlowCatalyst\Generated\Exception;

class PatchApiAdminEventTypeByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse400
     */
    private $apiAdminEventTypesIdPatchResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse400 $apiAdminEventTypesIdPatchResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdPatchResponse400 = $apiAdminEventTypesIdPatchResponse400;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdPatchResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse400
    {
        return $this->apiAdminEventTypesIdPatchResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}