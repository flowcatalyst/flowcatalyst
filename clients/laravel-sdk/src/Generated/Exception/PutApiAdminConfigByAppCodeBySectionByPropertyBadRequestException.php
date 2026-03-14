<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400
     */
    private $apiAdminConfigAppCodeSectionPropertyPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400 $apiAdminConfigAppCodeSectionPropertyPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyPutResponse400 = $apiAdminConfigAppCodeSectionPropertyPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse400
    {
        return $this->apiAdminConfigAppCodeSectionPropertyPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}