<?php

namespace FlowCatalyst\Generated\Exception;

class EnableApplicationForClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\NotFoundResponse
     */
    private $notFoundResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\NotFoundResponse $notFoundResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Application or client not found');
        $this->notFoundResponse = $notFoundResponse;
        $this->response = $response;
    }
    public function getNotFoundResponse(): \FlowCatalyst\Generated\Model\NotFoundResponse
    {
        return $this->notFoundResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}