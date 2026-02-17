<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdArchiveNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse404
     */
    private $bffEventTypesIdArchivePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse404 $bffEventTypesIdArchivePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdArchivePostResponse404 = $bffEventTypesIdArchivePostResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdArchivePostResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse404
    {
        return $this->bffEventTypesIdArchivePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}