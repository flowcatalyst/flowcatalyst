<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminRolesBySourceBySourceBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse400
     */
    private $apiAdminRolesBySourceSourceGetResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse400 $apiAdminRolesBySourceSourceGetResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesBySourceSourceGetResponse400 = $apiAdminRolesBySourceSourceGetResponse400;
        $this->response = $response;
    }
    public function getApiAdminRolesBySourceSourceGetResponse400(): \FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse400
    {
        return $this->apiAdminRolesBySourceSourceGetResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}