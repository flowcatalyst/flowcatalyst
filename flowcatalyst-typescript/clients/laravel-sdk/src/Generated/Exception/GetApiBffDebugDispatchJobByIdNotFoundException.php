<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffDebugDispatchJobByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse404
     */
    private $apiBffDebugDispatchJobsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse404 $apiBffDebugDispatchJobsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffDebugDispatchJobsIdGetResponse404 = $apiBffDebugDispatchJobsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiBffDebugDispatchJobsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse404
    {
        return $this->apiBffDebugDispatchJobsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}