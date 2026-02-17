<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffDispatchJobByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse404
     */
    private $apiBffDispatchJobsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse404 $apiBffDispatchJobsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffDispatchJobsIdGetResponse404 = $apiBffDispatchJobsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiBffDispatchJobsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse404
    {
        return $this->apiBffDispatchJobsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}