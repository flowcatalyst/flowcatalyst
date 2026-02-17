<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffDispatchJobsByIdAttemptNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse404
     */
    private $bffDispatchJobsIdAttemptsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse404 $bffDispatchJobsIdAttemptsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffDispatchJobsIdAttemptsGetResponse404 = $bffDispatchJobsIdAttemptsGetResponse404;
        $this->response = $response;
    }
    public function getBffDispatchJobsIdAttemptsGetResponse404(): \FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse404
    {
        return $this->bffDispatchJobsIdAttemptsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}