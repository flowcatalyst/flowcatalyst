<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdArchiveNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse404
     */
    private $apiBffEventTypesIdArchivePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse404 $apiBffEventTypesIdArchivePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdArchivePostResponse404 = $apiBffEventTypesIdArchivePostResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdArchivePostResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse404
    {
        return $this->apiBffEventTypesIdArchivePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}