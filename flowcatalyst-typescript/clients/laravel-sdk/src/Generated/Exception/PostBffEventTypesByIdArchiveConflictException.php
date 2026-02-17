<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdArchiveConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse409
     */
    private $bffEventTypesIdArchivePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse409 $bffEventTypesIdArchivePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdArchivePostResponse409 = $bffEventTypesIdArchivePostResponse409;
        $this->response = $response;
    }
    public function getBffEventTypesIdArchivePostResponse409(): \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse409
    {
        return $this->bffEventTypesIdArchivePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}