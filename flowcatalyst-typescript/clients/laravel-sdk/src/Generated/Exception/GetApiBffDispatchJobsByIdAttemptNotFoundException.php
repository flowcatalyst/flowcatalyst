<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffDispatchJobsByIdAttemptNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse404
     */
    private $apiBffDispatchJobsIdAttemptsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse404 $apiBffDispatchJobsIdAttemptsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffDispatchJobsIdAttemptsGetResponse404 = $apiBffDispatchJobsIdAttemptsGetResponse404;
        $this->response = $response;
    }
    public function getApiBffDispatchJobsIdAttemptsGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse404
    {
        return $this->apiBffDispatchJobsIdAttemptsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}