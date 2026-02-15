<?php

namespace FlowCatalyst\Generated\Exception;

class CreateRoleConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ConflictResponse
     */
    private $conflictResponse;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ConflictResponse $conflictResponse, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Role already exists');
        $this->conflictResponse = $conflictResponse;
        $this->response = $response;
    }
    public function getConflictResponse(): \FlowCatalyst\Generated\Model\ConflictResponse
    {
        return $this->conflictResponse;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}