<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffDebugDispatchJobByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse404
     */
    private $bffDebugDispatchJobsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse404 $bffDebugDispatchJobsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffDebugDispatchJobsIdGetResponse404 = $bffDebugDispatchJobsIdGetResponse404;
        $this->response = $response;
    }
    public function getBffDebugDispatchJobsIdGetResponse404(): \FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse404
    {
        return $this->bffDebugDispatchJobsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}