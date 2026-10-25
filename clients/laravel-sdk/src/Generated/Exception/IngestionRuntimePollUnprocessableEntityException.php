<?php

namespace FlowCatalyst\Generated\Exception;

class IngestionRuntimePollUnprocessableEntityException extends UnprocessableEntityException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ErrorModel
     */
    private $errorModel;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ErrorModel $errorModel, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Unprocessable Entity');
        $this->errorModel = $errorModel;
        $this->response = $response;
    }
    public function getErrorModel(): \FlowCatalyst\Generated\Model\ErrorModel
    {
        return $this->errorModel;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}