<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminAuditLogByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse404
     */
    private $apiAdminAuditLogsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse404 $apiAdminAuditLogsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuditLogsIdGetResponse404 = $apiAdminAuditLogsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuditLogsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse404
    {
        return $this->apiAdminAuditLogsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}