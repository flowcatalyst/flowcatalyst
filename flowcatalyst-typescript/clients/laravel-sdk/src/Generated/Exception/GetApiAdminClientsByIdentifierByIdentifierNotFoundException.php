<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminClientsByIdentifierByIdentifierNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404
     */
    private $apiAdminClientsByIdentifierIdentifierGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404 $apiAdminClientsByIdentifierIdentifierGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsByIdentifierIdentifierGetResponse404 = $apiAdminClientsByIdentifierIdentifierGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsByIdentifierIdentifierGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse404
    {
        return $this->apiAdminClientsByIdentifierIdentifierGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}