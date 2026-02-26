<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminDispatchJobByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse404
     */
    private $apiAdminDispatchJobsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse404 $apiAdminDispatchJobsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchJobsIdGetResponse404 = $apiAdminDispatchJobsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchJobsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse404
    {
        return $this->apiAdminDispatchJobsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}