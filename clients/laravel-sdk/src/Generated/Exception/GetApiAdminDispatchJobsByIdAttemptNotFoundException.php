<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminDispatchJobsByIdAttemptNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse404
     */
    private $apiAdminDispatchJobsIdAttemptsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse404 $apiAdminDispatchJobsIdAttemptsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchJobsIdAttemptsGetResponse404 = $apiAdminDispatchJobsIdAttemptsGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchJobsIdAttemptsGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse404
    {
        return $this->apiAdminDispatchJobsIdAttemptsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}