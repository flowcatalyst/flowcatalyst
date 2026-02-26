<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminDispatchPoolByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse400
     */
    private $apiAdminDispatchPoolsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse400 $apiAdminDispatchPoolsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdPutResponse400 = $apiAdminDispatchPoolsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse400
    {
        return $this->apiAdminDispatchPoolsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}