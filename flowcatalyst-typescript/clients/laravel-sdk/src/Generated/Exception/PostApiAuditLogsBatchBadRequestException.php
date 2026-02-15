<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAuditLogsBatchBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ErrorResponse5
     */
    private $errorResponse5;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ErrorResponse5 $errorResponse5, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Invalid request or batch size exceeds limit');
        $this->errorResponse5 = $errorResponse5;
        $this->response = $response;
    }
    public function getErrorResponse5(): \FlowCatalyst\Generated\Model\ErrorResponse5
    {
        return $this->errorResponse5;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}