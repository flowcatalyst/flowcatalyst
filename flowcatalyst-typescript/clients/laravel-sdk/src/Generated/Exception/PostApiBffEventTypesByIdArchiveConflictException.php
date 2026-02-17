<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdArchiveConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse409
     */
    private $apiBffEventTypesIdArchivePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse409 $apiBffEventTypesIdArchivePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdArchivePostResponse409 = $apiBffEventTypesIdArchivePostResponse409;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdArchivePostResponse409(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse409
    {
        return $this->apiBffEventTypesIdArchivePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}