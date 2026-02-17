<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffDispatchJobByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse404
     */
    private $bffDispatchJobsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse404 $bffDispatchJobsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffDispatchJobsIdGetResponse404 = $bffDispatchJobsIdGetResponse404;
        $this->response = $response;
    }
    public function getBffDispatchJobsIdGetResponse404(): \FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse404
    {
        return $this->bffDispatchJobsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}