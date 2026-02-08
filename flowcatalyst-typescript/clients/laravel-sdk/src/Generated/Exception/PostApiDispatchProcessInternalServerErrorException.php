<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiDispatchProcessInternalServerErrorException extends InternalServerErrorException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ProcessResponse
     */
    private $processResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ProcessResponse $processResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Internal error during processing');
        $this->processResponse = $processResponse;
        $this->response = $response;
    }
    public function getProcessResponse(): \FlowCatalyst\Generated\Model\ProcessResponse
    {
        return $this->processResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}